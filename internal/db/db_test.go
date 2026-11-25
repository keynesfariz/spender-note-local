package db

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestOpenDatabase_FreshSchemaDefaultsAndIndexes(t *testing.T) {
	gdb, err := OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open fresh db: %v", err)
	}

	var s AppSetting
	if err := gdb.First(&s, 1).Error; err != nil {
		t.Fatalf("load app_settings: %v", err)
	}
	if s.LlmProvider != "openai_compatible" {
		t.Fatalf("expected llm_provider=openai_compatible, got %q", s.LlmProvider)
	}
	if s.LlmBaseURL != DefaultLlmBaseURL {
		t.Fatalf("expected llm_base_url=%q, got %q", DefaultLlmBaseURL, s.LlmBaseURL)
	}
	if s.LlmModel != nil {
		t.Fatalf("expected llm_model=nil, got %q", *s.LlmModel)
	}

	var mig struct {
		Version int  `gorm:"column:version"`
		Dirty   bool `gorm:"column:dirty"`
	}
	if err := gdb.Raw(`SELECT version, dirty FROM schema_migrations`).Scan(&mig).Error; err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if mig.Version != baselineVersion || mig.Dirty {
		t.Fatalf("expected schema_migrations version=%d dirty=false, got version=%d dirty=%v", baselineVersion, mig.Version, mig.Dirty)
	}

	for _, idxName := range []string{
		"idx_transactions_currency_occurred",
		"idx_raw_emails_sender_date",
		"idx_transaction_tags_tag",
		"idx_transactions_category_occurred",
		"idx_email_extractions_triage_state_occurred",
		"idx_email_extractions_triage_signature_state",
	} {
		var found string
		if err := gdb.Raw(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, idxName).Scan(&found).Error; err != nil {
			t.Fatalf("check index %s: %v", idxName, err)
		}
		if found != idxName {
			t.Fatalf("expected index %s to exist", idxName)
		}
	}

	var cats []Category
	if err := gdb.Order("is_system ASC, sort_order ASC").Find(&cats).Error; err != nil {
		t.Fatalf("load seeded categories: %v", err)
	}
	if len(cats) != 11 {
		t.Fatalf("expected 11 seeded categories, got %d", len(cats))
	}
	if cats[0].Name != "Housing" || cats[0].Icon != "Home" || cats[0].IsSystem != 0 {
		t.Fatalf("unexpected first seeded category: %+v", cats[0])
	}
	last := cats[len(cats)-1]
	if last.Name != "Other" || last.Icon != "Ellipsis" || last.IsSystem != 1 {
		t.Fatalf("expected 'Other' as read-only system category at end, got %+v", last)
	}

	transaction := Transaction{
		ID: "tx1", SourceType: "manual", CategoryID: "missing", AmountMinor: 100,
		Currency: "USD", Merchant: "Store", OccurredAtUtc: "2026-11-23T00:00:00Z",
		CreatedAtUtc: "2026-11-23T00:00:00Z", UpdatedAtUtc: "2026-11-23T00:00:00Z",
	}
	if err := gdb.Create(&transaction).Error; err == nil || !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("expected transaction with an unknown category ID to fail the foreign key check, got %v", err)
	}
	transaction.CategoryID = last.ID
	if err := gdb.Create(&transaction).Error; err != nil {
		t.Fatalf("insert transaction with seeded category: %v", err)
	}
	if err := gdb.Delete(&last).Error; err == nil || !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("expected deleting a linked category to fail the foreign key check, got %v", err)
	}

	run := SyncRun{
		ID:           "run-syncing-1",
		Status:       "running",
		Phase:        "syncing",
		LlmProvider:  "openai_compatible",
		LlmModel:     "qwen2.5",
		StartedAtUtc: "2026-11-23T00:00:00Z",
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatalf("expected sync_runs to accept phase='syncing', got error: %v", err)
	}
}

func TestOpenDatabase_ConcurrentStartupMigrationsSerializeCleanly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")

	const workers = 6
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gdb, err := OpenDatabase(dbPath, false)
			if err != nil {
				errs <- err
				return
			}
			if sqlDB, err := gdb.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent OpenDatabase failed: %v", err)
	}
}

func TestOpenDatabase_PreservesExistingDirectoryPermissions(t *testing.T) {
	for name, dsn := range map[string]func(string) string{
		"plain": func(path string) string { return path },
		"query": func(path string) string { return path + "?_pragma=busy_timeout(5000)" },
		"URI": func(path string) string {
			return (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=rwc"}).String()
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "ledger file.db")
			gdb, err := OpenDatabase(dsn(path), false)
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := gdb.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			for path, want := range map[string]os.FileMode{
				dir: 0o755, path: 0o600, path + "-wal": 0o600,
				path + "-shm": 0o600, path + ".migrate.lock": 0o600,
			} {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != want {
					t.Errorf("%s mode = %04o, want %04o", path, got, want)
				}
			}
		})
	}
}

func TestOpenDatabase_FreshTriageConstraints(t *testing.T) {
	gdb, err := OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatal(err)
	}

	if err := gdb.Create(&SenderTriagePolicy{
		ID:             "pol-1",
		SenderEmail:    "bca@bca.co.id",
		SignatureKey:   "sig-1",
		SubjectPattern: "internet transaction journal",
		BodyCue:        "transaction type: qris payment",
		Action:         "admit",
		CreatedAtUtc:   "2026-11-24T12:00:00Z",
		UpdatedAtUtc:   "2026-11-24T12:00:00Z",
	}).Error; err != nil {
		t.Fatalf("insert valid sender_triage_policies row failed: %v", err)
	}

	if err := gdb.Create(&SenderTriagePolicy{
		ID:             "pol-dup",
		SenderEmail:    "bca@bca.co.id",
		SignatureKey:   "sig-1",
		SubjectPattern: "internet transaction journal",
		BodyCue:        "transaction type: qris payment",
		Action:         "ignore",
		CreatedAtUtc:   "2026-11-24T12:00:00Z",
		UpdatedAtUtc:   "2026-11-24T12:00:00Z",
	}).Error; err == nil {
		t.Fatalf("expected UNIQUE (sender_email, signature_key) failure, got nil")
	}

	if err := gdb.Create(&SenderTriagePolicy{
		ID:             "pol-bad-action",
		SenderEmail:    "bca@bca.co.id",
		SignatureKey:   "sig-2",
		SubjectPattern: "internet transaction journal",
		Action:         "invalid_action",
		CreatedAtUtc:   "2026-11-24T12:00:00Z",
		UpdatedAtUtc:   "2026-11-24T12:00:00Z",
	}).Error; err == nil {
		t.Fatalf("expected CHECK constraint failure on invalid action, got nil")
	}

	if err := gdb.Exec(`
		INSERT INTO raw_emails (id, gmail_message_id, from_address, subject, cleaned_text, internal_date_ms, received_at_utc, status)
		VALUES ('e-1', 'gm-1', 'bca@bca.co.id', 'Subj', 'Body', 1000, '2026-11-24T12:00:00Z', 'non_transaction')
	`).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`
		INSERT INTO email_extractions (id, email_id, email_kind, extraction_model, raw_llm_json, triage_state)
		VALUES ('ext-bad', 'e-1', 'non_transaction', 'model', '{}', 'invalid_state')
	`).Error; err == nil {
		t.Fatalf("expected CHECK constraint failure on email_extractions.triage_state, got nil")
	}
}

func TestRunImmediateTx_CommitAndRollback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immediate-tx.db")
	gdb, err := OpenDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	err = RunImmediateTx(gdb, func(tx *gorm.DB) error {
		if err := tx.Create(&SenderTriagePolicy{
			ID:             "pol-commit",
			SenderEmail:    "alerts@bank.test",
			SignatureKey:   "sig-commit",
			SubjectPattern: "alert",
			BodyCue:        "",
			Action:         "admit",
			CreatedAtUtc:   "2026-11-24T12:00:00Z",
			UpdatedAtUtc:   "2026-11-24T12:00:00Z",
		}).Error; err != nil {
			return err
		}
		return tx.Model(&SenderTriagePolicy{}).Where("id = ?", "pol-commit").Updates(map[string]any{
			"body_cue": "updated cue",
		}).Error
	})
	if err != nil {
		t.Fatalf("RunImmediateTx commit path failed: %v", err)
	}

	var stored SenderTriagePolicy
	if err := gdb.First(&stored, "id = ?", "pol-commit").Error; err != nil {
		t.Fatalf("load committed policy: %v", err)
	}
	if stored.BodyCue != "updated cue" {
		t.Fatalf("expected body_cue 'updated cue', got %q", stored.BodyCue)
	}

	errRollback := RunImmediateTx(gdb, func(tx *gorm.DB) error {
		if err := tx.Create(&SenderTriagePolicy{
			ID:             "pol-rollback",
			SenderEmail:    "alerts@bank.test",
			SignatureKey:   "sig-rollback",
			SubjectPattern: "alert",
			Action:         "ignore",
			CreatedAtUtc:   "2026-11-24T12:00:00Z",
			UpdatedAtUtc:   "2026-11-24T12:00:00Z",
		}).Error; err != nil {
			return err
		}
		return os.ErrInvalid
	})
	if errRollback == nil {
		t.Fatalf("expected error from rollback RunImmediateTx")
	}

	var rollbackCount int64
	if err := gdb.Model(&SenderTriagePolicy{}).Where("id = ?", "pol-rollback").Count(&rollbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatalf("expected rolled-back row count 0, got %d", rollbackCount)
	}
}
