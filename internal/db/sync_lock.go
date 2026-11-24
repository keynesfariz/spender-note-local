package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"gorm.io/gorm"
)

// TryLockSync serializes sync jobs and recovery across processes sharing a
// database. A nil release function means another process owns the lock.
func TryLockSync(gdb *gorm.DB) (release func(), err error) {
	var databases []struct {
		Name string
		File string
	}
	if err := gdb.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		return nil, err
	}
	for _, database := range databases {
		if database.Name != "main" || database.File == "" {
			continue
		}
		path, err := filepath.EvalSymlinks(database.File)
		if err != nil {
			return nil, fmt.Errorf("resolve sync database path: %w", err)
		}
		file, err := os.OpenFile(path+".sync.lock", os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open sync lock: %w", err)
		}
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = file.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, nil
			}
			return nil, fmt.Errorf("acquire sync lock: %w", err)
		}
		return func() { _ = file.Close() }, nil
	}
	// In-memory databases have no filesystem path. The conditional sync-run
	// insertion still serializes jobs sharing the same SQLite database.
	return func() {}, nil
}

func recoverInterruptedSyncRuns(gdb *gorm.DB) error {
	release, err := TryLockSync(gdb)
	if err != nil || release == nil {
		return err
	}
	defer release()

	nowUtc := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return gdb.Exec(`
		UPDATE sync_runs
		SET status = 'interrupted',
		    phase = 'done',
		    error_message = COALESCE(error_message, 'Server restarted while sync was running'),
		    finished_at_utc = COALESCE(finished_at_utc, ?)
		WHERE status = 'running'`, nowUtc).Error
}
