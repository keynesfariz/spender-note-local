package domain

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// TriageBackfill remembers successful readiness for one route process and database.
type TriageBackfill struct {
	mu    sync.Mutex
	ready bool
}

func (backfill *TriageBackfill) ensure(gdb *gorm.DB) error {
	// The compatibility write uses an explicit SQLite transaction. It must be
	// able to commit or roll back even if the request that started it goes away.
	if backfill == nil {
		return BackfillSkippedEmailTriageData(gdb.WithContext(context.Background()))
	}

	backfill.mu.Lock()
	defer backfill.mu.Unlock()
	if backfill.ready {
		return nil
	}
	if err := BackfillSkippedEmailTriageData(gdb.WithContext(context.Background())); err != nil {
		return err
	}
	backfill.ready = true
	return nil
}
