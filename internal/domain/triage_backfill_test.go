package domain

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"portspendernote/internal/db"
)

type triageBackfillTrace struct {
	logger.Interface
	checks  atomic.Int32
	onBegin context.CancelFunc
}

func (trace *triageBackfillTrace) Trace(ctx context.Context, start time.Time, statement func() (string, int64), err error) {
	sql, _ := statement()
	if strings.Contains(sql, "SELECT 1") && strings.Contains(sql, "triage_signature_key IS NULL") {
		trace.checks.Add(1)
	}
	if sql == "BEGIN IMMEDIATE" && trace.onBegin != nil {
		trace.onBegin()
	}
	trace.Interface.Trace(ctx, start, statement, err)
}

func TestTriageBackfill_ConcurrentReadAndWarmPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "triage.sqlite")
	left, err := db.OpenDatabase(path, false)
	if err != nil {
		t.Fatal(err)
	}
	right, err := db.OpenDatabase(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateLocaleSettings(left, UpdateLocaleInput{CountryCode: "ID"}); err != nil {
		t.Fatal(err)
	}
	seedLegacySkippedEmail(t, left, "legacy", nil, "wallet@example.test", "Transfer receipt",
		"Money transferred", 1795524000000, "2026-11-24T12:40:00Z", "legacy",
		`{"email_kind":"non_transaction","amount_decimal":"45000","currency":"IDR","merchant":"Example"}`)

	type result struct {
		count int64
		err   error
	}
	both := make(chan result, 2)
	leftGate, rightGate := &TriageBackfill{}, &TriageBackfill{}
	leftTrace := &triageBackfillTrace{Interface: logger.Default}
	left = left.Session(&gorm.Session{Logger: leftTrace})
	go func() {
		list, err := ListTransactions(left, ListTransactionsParams{Backfill: leftGate})
		if err != nil {
			both <- result{err: err}
			return
		}
		both <- result{count: list.TotalGhostCount}
	}()
	go func() {
		list, err := ListSkippedTransactionsWithBackfill(right, 1, 25, rightGate)
		if err != nil {
			both <- result{err: err}
			return
		}
		both <- result{count: list.TotalCount}
	}()
	for range 2 {
		got := <-both
		if got.err != nil || got.count != 1 {
			t.Fatalf("concurrent first read = %+v", got)
		}
	}

	firstChecks := leftTrace.checks.Load()
	if firstChecks == 0 {
		t.Fatal("backfill readiness was not checked")
	}
	if _, err := ListTransactions(left, ListTransactionsParams{Backfill: leftGate}); err != nil {
		t.Fatal(err)
	}
	if leftTrace.checks.Load() != firstChecks {
		t.Fatal("warm read performed a backfill readiness query")
	}
}

func TestTriageBackfill_RetryAfterCanceledRequest(t *testing.T) {
	gdb := openDomainTestDB(t)
	seedLegacySkippedEmail(t, gdb, "legacy", nil, "wallet@example.test", "Transfer receipt",
		"Money transferred", 1795524000000, "2026-11-24T12:40:00Z", "legacy",
		`{"email_kind":"non_transaction","amount_decimal":"45000","currency":"IDR","merchant":"Example"}`)
	if err := gdb.Exec(`CREATE TRIGGER fail_legacy_backfill BEFORE UPDATE ON email_extractions
		BEGIN SELECT RAISE(ABORT, 'forced backfill error'); END`).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trace := &triageBackfillTrace{Interface: logger.Default, onBegin: cancel}
	requestDB := gdb.Session(&gorm.Session{Logger: trace}).WithContext(ctx)
	gate := &TriageBackfill{}
	if _, err := ListSkippedTransactionsWithBackfill(requestDB, 1, 25, gate); err == nil || !strings.Contains(err.Error(), "forced backfill error") || ctx.Err() == nil {
		t.Fatalf("expected write error during canceled request, got %v", err)
	}
	if err := gdb.Exec("DROP TRIGGER fail_legacy_backfill").Error; err != nil {
		t.Fatalf("connection unusable after backfill rollback: %v", err)
	}
	list, err := ListSkippedTransactionsWithBackfill(gdb, 1, 25, gate)
	if err != nil || list.TotalCount != 1 {
		t.Fatalf("retry = %+v, %v", list, err)
	}
}
