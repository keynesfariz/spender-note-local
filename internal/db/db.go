package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const DefaultLlmBaseURL = "http://localhost:9931/v1"

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	singletonOnce sync.Once
	singletonDB   *gorm.DB
	singletonErr  error
)

func isMemoryDB(dbPath string) bool {
	return dbPath == ":memory:" || strings.HasPrefix(dbPath, "file::memory:")
}

func enforceOwnerPermissions(dbPath string) {
	if isMemoryDB(dbPath) {
		return
	}
	dir := filepath.Dir(dbPath)
	_ = os.MkdirAll(dir, 0o700)
	_ = os.Chmod(dir, 0o700)
	for _, suffix := range []string{"", "-wal", "-shm", ".migrate.lock"} {
		filePath := dbPath + suffix
		if _, err := os.Stat(filePath); err == nil {
			_ = os.Chmod(filePath, 0o600)
		}
	}
}

func withMigrationLock(dbPath string, fn func() error) error {
	if isMemoryDB(dbPath) {
		return fn()
	}
	lockPath := dbPath + ".migrate.lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open migration lock file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire migration flock: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	return fn()
}

func tableExists(sqlDB *sql.DB, tableName string) (bool, error) {
	var name string
	err := sqlDB.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", tableName, err)
	}
	return true, nil
}

func runMigrations(sqlDB *sql.DB) error {
	hasSchemaMigrations, err := tableExists(sqlDB, "schema_migrations")
	if err != nil {
		return err
	}
	needsBaseline := false
	if !hasSchemaMigrations {
		hasAppSettings, err := tableExists(sqlDB, "app_settings")
		if err != nil {
			return err
		}
		needsBaseline = hasAppSettings
	}

	srcDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("init embedded migration source: %w", err)
	}
	defer srcDriver.Close()

	dbDriver, err := newSQLiteMigrateDriver(sqlDB)
	if err != nil {
		return fmt.Errorf("init sqlite migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", srcDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if needsBaseline {
		if err := m.Force(1); err != nil {
			return fmt.Errorf("baseline existing database at v1: %w", err)
		}
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func OpenDatabase(dbPath string, recoverStaleRuns bool) (*gorm.DB, error) {
	if !isMemoryDB(dbPath) {
		enforceOwnerPermissions(dbPath)
	}

	var gdb *gorm.DB
	err := withMigrationLock(dbPath, func() error {
		dsn := dbPath
		if !isMemoryDB(dbPath) && !strings.Contains(dbPath, "?") {
			dsn = dbPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
		}

		opened, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return fmt.Errorf("open sqlite database: %w", err)
		}

		sqlDB, err := opened.DB()
		if err != nil {
			return fmt.Errorf("get underlying sql.DB: %w", err)
		}
		// SQLite works best with a single writer connection in WAL mode.
		sqlDB.SetMaxOpenConns(1)

		if err := opened.Exec("PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;").Error; err != nil {
			_ = sqlDB.Close()
			return fmt.Errorf("configure sqlite pragmas: %w", err)
		}

		if err := runMigrations(sqlDB); err != nil {
			_ = sqlDB.Close()
			return fmt.Errorf("migrate schema: %w", err)
		}

		if err := opened.Exec("INSERT OR IGNORE INTO app_settings (id) VALUES (1)").Error; err != nil {
			_ = sqlDB.Close()
			return fmt.Errorf("seed app_settings: %w", err)
		}
		if err := opened.Exec("INSERT OR IGNORE INTO gmail_oauth (id, status) VALUES (1, 'disconnected')").Error; err != nil {
			_ = sqlDB.Close()
			return fmt.Errorf("seed gmail_oauth: %w", err)
		}

		if recoverStaleRuns {
			nowUtc := time.Now().UTC().Format("2006-01-02T15:04:05Z")
			_ = opened.Exec(`
				UPDATE sync_runs
				SET status = 'interrupted',
				    phase = 'done',
				    error_message = COALESCE(error_message, 'Server restarted while sync was running'),
				    finished_at_utc = COALESCE(finished_at_utc, ?)
				WHERE status = 'running'`, nowUtc).Error
		}

		if !isMemoryDB(dbPath) {
			enforceOwnerPermissions(dbPath)
		}

		gdb = opened
		return nil
	})
	if err != nil {
		return nil, err
	}
	return gdb, nil
}

func GetDB() (*gorm.DB, error) {
	singletonOnce.Do(func() {
		dbPath := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
		if dbPath == "" {
			dbPath = "data/spender.db"
		}
		singletonDB, singletonErr = OpenDatabase(dbPath, false)
	})
	return singletonDB, singletonErr
}

// GetDBWithRecovery is called by the sync route process on boot to recover any
// stale running sync_runs left over from a previous server restart.
func GetDBWithRecovery() (*gorm.DB, error) {
	singletonOnce.Do(func() {
		dbPath := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
		if dbPath == "" {
			dbPath = "data/spender.db"
		}
		singletonDB, singletonErr = OpenDatabase(dbPath, true)
	})
	return singletonDB, singletonErr
}
