package db

import (
	"database/sql"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/golang-migrate/migrate/v4/database"
)

const defaultMigrationsTable = "schema_migrations"

// sqliteMigrateDriver implements golang-migrate's database.Driver directly on
// an existing *sql.DB without importing modernc.org/sqlite (which would collide
// with github.com/glebarez/go-sqlite's sql.Register("sqlite", ...) in init).
type sqliteMigrateDriver struct {
	db     *sql.DB
	locked atomic.Bool
}

func newSQLiteMigrateDriver(db *sql.DB) (*sqliteMigrateDriver, error) {
	d := &sqliteMigrateDriver{db: db}
	if err := d.ensureVersionTable(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *sqliteMigrateDriver) Open(url string) (database.Driver, error) {
	return nil, fmt.Errorf("not implemented: use newSQLiteMigrateDriver with an existing *sql.DB")
}

func (d *sqliteMigrateDriver) Close() error {
	return nil
}

func (d *sqliteMigrateDriver) Lock() error {
	if !d.locked.CompareAndSwap(false, true) {
		return database.ErrLocked
	}
	return nil
}

func (d *sqliteMigrateDriver) Unlock() error {
	if !d.locked.CompareAndSwap(true, false) {
		return database.ErrNotLocked
	}
	return nil
}

func (d *sqliteMigrateDriver) ensureVersionTable() error {
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS ` + defaultMigrationsTable + ` (version uint64, dirty bool); CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON ` + defaultMigrationsTable + ` (version);`)
	if err != nil {
		return fmt.Errorf("ensure %s table: %w", defaultMigrationsTable, err)
	}
	return nil
}

func (d *sqliteMigrateDriver) Run(migration io.Reader) error {
	body, err := io.ReadAll(migration)
	if err != nil {
		return err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(string(body)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (d *sqliteMigrateDriver) SetVersion(version int, dirty bool) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ` + defaultMigrationsTable); err != nil {
		_ = tx.Rollback()
		return err
	}
	if version >= 0 || (version == database.NilVersion && dirty) {
		if _, err := tx.Exec(`INSERT INTO `+defaultMigrationsTable+` (version, dirty) VALUES (?, ?)`, version, dirty); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (d *sqliteMigrateDriver) Version() (int, bool, error) {
	var version int
	var dirty bool
	err := d.db.QueryRow(`SELECT version, dirty FROM ` + defaultMigrationsTable + ` LIMIT 1`).Scan(&version, &dirty)
	if err == sql.ErrNoRows {
		return database.NilVersion, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

func (d *sqliteMigrateDriver) Drop() error {
	return fmt.Errorf("drop not supported")
}
