package db

import (
	"strings"
	"testing"
)

func TestFreshBaseline_RejectsMissingSenderDefaultsWithForeignKeysOff(t *testing.T) {
	gdb, err := OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := gdb.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatal(err)
	}
	const now = "2026-11-25T00:00:00Z"
	if err := gdb.Create(&Account{
		ID: "valid", Name: "Test", Type: "debit_card", Currency: "IDR",
		InitialBalanceAsOfUtc: now, CreatedAtUtc: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	missing, valid := "missing", "valid"
	sender := Sender{
		ID: "sender", Email: "sender@example.com", RoleHint: "auto", Enabled: 1,
		BackfillFromUtc: now, DefaultAccountID: &valid,
	}
	assertMissingAccountError := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "sender default account does not exist") {
			t.Fatalf("expected missing sender default error, got %v", err)
		}
	}
	t.Run("insert", func(t *testing.T) {
		invalid := sender
		invalid.ID = "invalid"
		invalid.Email = "invalid@example.com"
		invalid.DefaultAccountID = &missing
		assertMissingAccountError(t, gdb.Create(&invalid).Error)
	})
	if err := gdb.Create(&sender).Error; err != nil {
		t.Fatal(err)
	}
	t.Run("update", func(t *testing.T) {
		assertMissingAccountError(t, gdb.Model(&sender).Update("default_account_id", missing).Error)
	})
	if err := gdb.Model(&sender).Update("default_account_id", nil).Error; err != nil {
		t.Fatalf("clear sender default: %v", err)
	}
}
