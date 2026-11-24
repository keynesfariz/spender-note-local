package db

import (
	"fmt"

	"gorm.io/gorm"
)

// RunImmediateTx executes fn inside a SQLite BEGIN IMMEDIATE transaction on a
// pinned connection so concurrent route processes acquire the write lock upfront
// without nesting GORM's default DEFERRED transactions.
func RunImmediateTx(gdb *gorm.DB, fn func(tx *gorm.DB) error) error {
	return gdb.Connection(func(connDB *gorm.DB) error {
		tx := connDB.Session(&gorm.Session{SkipDefaultTransaction: true})
		if err := tx.Exec("BEGIN IMMEDIATE").Error; err != nil {
			return fmt.Errorf("begin immediate tx: %w", err)
		}

		committed := false
		defer func() {
			if !committed {
				_ = tx.Exec("ROLLBACK").Error
			}
		}()

		if err := fn(tx); err != nil {
			return err
		}

		if err := tx.Exec("COMMIT").Error; err != nil {
			return fmt.Errorf("commit immediate tx: %w", err)
		}
		committed = true
		return nil
	})
}
