package domain

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"portspendernote/internal/db"
)

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func configureTestLocale(t *testing.T, gdb *gorm.DB, tz, currency string) {
	t.Helper()
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"country_code": "ID",
		"timezone":     tz,
		"currency":     currency,
	}).Error; err != nil {
		t.Fatalf("configure test locale: %v", err)
	}
}

func TestAccountStatementDay_CRUDAndValidation(t *testing.T) {
	gdb := openDomainTestDB(t)
	configureTestLocale(t, gdb, "Asia/Jakarta", "IDR")

	// Create credit card with statement day 22
	cc, err := CreateAccount(gdb, CreateAccountInput{
		Name:                "Jenius CC",
		Type:                "credit_card",
		InitialBalanceMinor: 0,
		StatementDay:        intPtr(22),
	})
	if err != nil {
		t.Fatalf("create credit card with statement_day=22: %v", err)
	}
	if cc.StatementDay == nil || *cc.StatementDay != 22 {
		t.Fatalf("expected statement_day=22, got %v", cc.StatementDay)
	}

	// Create credit card without statement day
	ccNil, err := CreateAccount(gdb, CreateAccountInput{
		Name:                "No Cycle CC",
		Type:                "credit_card",
		InitialBalanceMinor: 0,
	})
	if err != nil {
		t.Fatalf("create credit card without statement_day: %v", err)
	}
	if ccNil.StatementDay != nil {
		t.Fatalf("expected nil statement_day, got %v", *ccNil.StatementDay)
	}

	// Update: set to 15
	updated, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		HasStatementDay: true,
		StatementDay:    intPtr(15),
	})
	if err != nil {
		t.Fatalf("update statement_day=15: %v", err)
	}
	if updated.StatementDay == nil || *updated.StatementDay != 15 {
		t.Fatalf("expected statement_day=15, got %v", updated.StatementDay)
	}

	// Update: omit HasStatementDay keeps 15 intact
	renamed, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		Name: strPtr("Jenius Platinum"),
	})
	if err != nil {
		t.Fatalf("rename account without statement_day patch: %v", err)
	}
	if renamed.StatementDay == nil || *renamed.StatementDay != 15 {
		t.Fatalf("expected statement_day=15 to remain unchanged, got %v", renamed.StatementDay)
	}

	// Update: explicitly clear to nil
	cleared, err := UpdateAccount(gdb, cc.ID, UpdateAccountInput{
		HasStatementDay: true,
		StatementDay:    nil,
	})
	if err != nil {
		t.Fatalf("clear statement_day: %v", err)
	}
	if cleared.StatementDay != nil {
		t.Fatalf("expected cleared statement_day=nil, got %v", *cleared.StatementDay)
	}

	// Validation: reject 0 and 32
	for _, bad := range []int{0, -1, 32} {
		if _, err := CreateAccount(gdb, CreateAccountInput{
			Name:                "Bad CC",
			Type:                "credit_card",
			InitialBalanceMinor: 0,
			StatementDay:        intPtr(bad),
		}); err == nil {
			t.Fatalf("expected error for statement_day=%d", bad)
		}
	}

	// Validation: reject non-credit-card with statement_day
	for _, acctType := range []string{"debit_card", "digital_wallet"} {
		if _, err := CreateAccount(gdb, CreateAccountInput{
			Name:                "Non CC",
			Type:                acctType,
			InitialBalanceMinor: 0,
			StatementDay:        intPtr(22),
		}); err == nil {
			t.Fatalf("expected error for %s with statement_day", acctType)
		}
	}
}

func TestMigrateSchema_AddsStatementDayToExistingAccountsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	rawDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	legacyAccountsSQL := `
	CREATE TABLE accounts (
	  id TEXT PRIMARY KEY,
	  name TEXT NOT NULL,
	  type TEXT NOT NULL CHECK (type IN ('credit_card', 'debit_card', 'digital_wallet')),
	  currency TEXT NOT NULL CHECK (length(currency) = 3),
	  initial_balance_minor INTEGER,
	  initial_balance_as_of_utc TEXT NOT NULL CHECK (length(initial_balance_as_of_utc) = 20 AND initial_balance_as_of_utc LIKE '%Z'),
	  last4 TEXT CHECK (last4 IS NULL OR (length(last4) = 4 AND last4 GLOB '[0-9][0-9][0-9][0-9]')),
	  status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('provisional', 'confirmed')),
	  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
	) STRICT;`
	if err := rawDB.Exec(legacyAccountsSQL).Error; err != nil {
		t.Fatalf("seed legacy accounts table: %v", err)
	}
	sqlDB, _ := rawDB.DB()
	_ = sqlDB.Close()

	migrated, err := db.OpenDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("OpenDatabase on legacy db: %v", err)
	}
	configureTestLocale(t, migrated, "Asia/Jakarta", "IDR")

	acct, err := CreateAccount(migrated, CreateAccountInput{
		Name:                "Migrated CC",
		Type:                "credit_card",
		InitialBalanceMinor: 0,
		StatementDay:        intPtr(22),
	})
	if err != nil {
		t.Fatalf("create account after migration: %v", err)
	}
	if acct.StatementDay == nil || *acct.StatementDay != 22 {
		t.Fatalf("expected statement_day=22 after migration, got %v", acct.StatementDay)
	}
}
