package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

type accountBalanceInfo struct {
	ComputedBalanceMinor   *int64
	NetFlowMinor           int64
	UncountedForeignCount  int64
	FutureTransactionCount int64
	LatestReconciliation   *db.AccountReconciliation
	Currency               string
}

func resolveAppCurrencyAndTimezone(gdb *gorm.DB, targetCurrency string, targetTimezone string) (string, string, error) {
	currency := strings.TrimSpace(targetCurrency)
	tz := strings.TrimSpace(targetTimezone)
	if currency == "" || tz == "" {
		var s db.AppSetting
		err := gdb.First(&s, 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", err
		}
		if err == nil {
			if currency == "" && s.Currency != nil && *s.Currency != "" {
				currency = *s.Currency
			}
			if tz == "" && s.Timezone != nil && *s.Timezone != "" {
				tz = *s.Timezone
			}
		}
	}
	if tz == "" {
		tz = "UTC"
	}
	return currency, tz, nil
}

func computeAccountBalanceAsOf(gdb *gorm.DB, account db.Account, targetCurrency string, asOfUtc string, nowUtc string) (accountBalanceInfo, error) {
	settingsCurrency, _, err := resolveAppCurrencyAndTimezone(gdb, targetCurrency, "UTC")
	if err != nil {
		return accountBalanceInfo{}, err
	}
	if settingsCurrency == "" {
		settingsCurrency = account.Currency
	}

	var latestRec db.AccountReconciliation
	var latestRecPtr *db.AccountReconciliation
	err = gdb.Where("account_id = ? AND recorded_at_utc <= ?", account.ID, asOfUtc).
		Order("recorded_at_utc DESC, rowid DESC").
		First(&latestRec).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return accountBalanceInfo{}, err
	}
	if err == nil {
		latestRecPtr = &latestRec
	}

	var anchorBalance *int64
	var anchorTime string
	strictAfterAnchor := false
	if latestRecPtr != nil {
		b := latestRecPtr.ActualBalanceMinor
		anchorBalance = &b
		anchorTime = latestRecPtr.RecordedAtUtc
		strictAfterAnchor = true
	} else if account.InitialBalanceMinor != nil && account.InitialBalanceAsOfUtc <= asOfUtc {
		anchorBalance = account.InitialBalanceMinor
		anchorTime = account.InitialBalanceAsOfUtc
	}

	type aggResult struct {
		LocalTotal int64 `gorm:"column:local_total"`
		ForeignCnt int64 `gorm:"column:foreign_cnt"`
	}
	var agg aggResult
	if anchorBalance == nil {
		if err := gdb.Raw(`
			SELECT
				COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
				COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_cnt
			FROM transactions
			WHERE account_id = ? AND occurred_at_utc <= ?`,
			settingsCurrency, settingsCurrency, account.ID, asOfUtc,
		).Scan(&agg).Error; err != nil {
			return accountBalanceInfo{}, err
		}
	} else if strictAfterAnchor {
		if err := gdb.Raw(`
			SELECT
				COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
				COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_cnt
			FROM transactions
			WHERE account_id = ? AND occurred_at_utc > ? AND occurred_at_utc <= ?`,
			settingsCurrency, settingsCurrency, account.ID, anchorTime, asOfUtc,
		).Scan(&agg).Error; err != nil {
			return accountBalanceInfo{}, err
		}
	} else {
		if err := gdb.Raw(`
			SELECT
				COALESCE(SUM(CASE WHEN currency = ? THEN amount_minor ELSE 0 END), 0) AS local_total,
				COALESCE(SUM(CASE WHEN currency != ? THEN 1 ELSE 0 END), 0) AS foreign_cnt
			FROM transactions
			WHERE account_id = ? AND occurred_at_utc >= ? AND occurred_at_utc <= ?`,
			settingsCurrency, settingsCurrency, account.ID, anchorTime, asOfUtc,
		).Scan(&agg).Error; err != nil {
			return accountBalanceInfo{}, err
		}
	}

	var futureCnt int64
	if err := gdb.Model(&db.Transaction{}).
		Where("account_id = ? AND occurred_at_utc > ?", account.ID, nowUtc).
		Count(&futureCnt).Error; err != nil {
		return accountBalanceInfo{}, err
	}

	var netFlow int64
	if agg.LocalTotal != 0 {
		if account.Type == "credit_card" {
			netFlow = agg.LocalTotal
		} else {
			netFlow = -agg.LocalTotal
		}
	}

	var computed *int64
	if anchorBalance != nil {
		v := *anchorBalance + netFlow
		computed = &v
	}

	return accountBalanceInfo{
		ComputedBalanceMinor:   computed,
		NetFlowMinor:           netFlow,
		UncountedForeignCount:  agg.ForeignCnt,
		FutureTransactionCount: futureCnt,
		LatestReconciliation:   latestRecPtr,
		Currency:               settingsCurrency,
	}, nil
}

func computeAccountBalance(gdb *gorm.DB, account db.Account, targetCurrency string) (accountBalanceInfo, error) {
	nowUtc := NowUtcIsoSeconds()
	return computeAccountBalanceAsOf(gdb, account, targetCurrency, nowUtc, nowUtc)
}

func computeBillingCycleSummary(gdb *gorm.DB, account db.Account, targetCurrency string, tz string, now time.Time) (*CreditCardBillingCycleSummary, error) {
	if account.Type != "credit_card" || account.StatementDay == nil {
		return nil, nil
	}
	settingsCurrency, resolvedTz, err := resolveAppCurrencyAndTimezone(gdb, targetCurrency, tz)
	if err != nil {
		return nil, err
	}
	if settingsCurrency == "" {
		settingsCurrency = account.Currency
	}

	bounds, err := ComputeCreditCardCycleBounds(*account.StatementDay, resolvedTz, now)
	if err != nil {
		return nil, nil
	}

	type cycleAgg struct {
		SpentMinor int64 `gorm:"column:spent_minor"`
		TxCount    int64 `gorm:"column:tx_count"`
	}

	nowUtc := ToUtcIsoSeconds(now)

	var curAgg cycleAgg
	if err := gdb.Raw(`
		SELECT
			COALESCE(SUM(amount_minor), 0) AS spent_minor,
			COUNT(*) AS tx_count
		FROM transactions
		WHERE account_id = ? AND currency = ? AND occurred_at_utc >= ? AND occurred_at_utc < ? AND occurred_at_utc <= ?`,
		account.ID, settingsCurrency, bounds.CurrentCycle.StartUtc, bounds.CurrentCycle.EndExclusiveUtc, nowUtc,
	).Scan(&curAgg).Error; err != nil {
		return nil, err
	}

	var lastAgg cycleAgg
	if err := gdb.Raw(`
		SELECT
			COALESCE(SUM(amount_minor), 0) AS spent_minor,
			COUNT(*) AS tx_count
		FROM transactions
		WHERE account_id = ? AND currency = ? AND occurred_at_utc >= ? AND occurred_at_utc < ?`,
		account.ID, settingsCurrency, bounds.LastCycle.StartUtc, bounds.LastCycle.EndExclusiveUtc,
	).Scan(&lastAgg).Error; err != nil {
		return nil, err
	}

	return &CreditCardBillingCycleSummary{
		StatementDay:      *account.StatementDay,
		LastStatementDate: bounds.LastStatementDate,
		CurrentCycle: BillingCycleSpendSummary{
			StartDate:        bounds.CurrentCycle.StartDate,
			EndDate:          bounds.CurrentCycle.EndDate,
			SpentMinor:       curAgg.SpentMinor,
			TransactionCount: curAgg.TxCount,
		},
		LastCycle: BillingCycleSpendSummary{
			StartDate:        bounds.LastCycle.StartDate,
			EndDate:          bounds.LastCycle.EndDate,
			SpentMinor:       lastAgg.SpentMinor,
			TransactionCount: lastAgg.TxCount,
		},
	}, nil
}

func isAccountTypeLocked(gdb *gorm.DB, account db.Account, hasRec bool) (bool, error) {
	if hasRec {
		return true, nil
	}
	if account.Status == "provisional" {
		return false, nil
	}
	return hasAccountLedgerRows(gdb, account.ID)
}

func buildAccountSummaryAt(gdb *gorm.DB, account db.Account, targetCurrency string, targetTimezone string, identifiers []db.AccountIdentifier, now time.Time) (AccountSummary, error) {
	nowUtc := ToUtcIsoSeconds(now)
	bal, err := computeAccountBalanceAsOf(gdb, account, targetCurrency, nowUtc, nowUtc)
	if err != nil {
		return AccountSummary{}, err
	}
	if identifiers == nil {
		identifiers, err = listIdentifiersForAccount(gdb, account.ID)
		if err != nil {
			return AccountSummary{}, err
		}
	}
	billingCycle, err := computeBillingCycleSummary(gdb, account, bal.Currency, targetTimezone, now)
	if err != nil {
		return AccountSummary{}, err
	}
	typeLocked, err := isAccountTypeLocked(gdb, account, bal.LatestReconciliation != nil)
	if err != nil {
		return AccountSummary{}, err
	}
	return AccountSummary{
		Account:                account,
		Identifiers:            identifiers,
		ComputedBalanceMinor:   bal.ComputedBalanceMinor,
		NetFlowMinor:           bal.NetFlowMinor,
		UncountedForeignCount:  bal.UncountedForeignCount,
		FutureTransactionCount: bal.FutureTransactionCount,
		LatestReconciliation:   bal.LatestReconciliation,
		BillingCycle:           billingCycle,
		TypeLocked:             typeLocked,
	}, nil
}

func buildAccountSummary(gdb *gorm.DB, account db.Account, targetCurrency string, identifiers []db.AccountIdentifier) (AccountSummary, error) {
	return buildAccountSummaryAt(gdb, account, targetCurrency, "", identifiers, time.Now().UTC())
}

type ReconcileAccountInput struct {
	AccountID          string  `json:"account_id"`
	ActualBalanceMinor int64   `json:"actual_balance_minor"`
	Note               *string `json:"note"`
	RecordedAtUtc      *string `json:"recorded_at_utc"`
}

func ReconcileAccount(gdb *gorm.DB, input ReconcileAccountInput) (map[string]any, error) {
	return reconcileAccountAt(gdb, input, time.Now().UTC())
}

func reconcileAccountAt(gdb *gorm.DB, input ReconcileAccountInput, now time.Time) (map[string]any, error) {
	nowUtc := ToUtcIsoSeconds(now)
	recordedAt := nowUtc
	if input.RecordedAtUtc != nil && strings.TrimSpace(*input.RecordedAtUtc) != "" {
		recordedAt = strings.TrimSpace(*input.RecordedAtUtc)
	}
	if !IsValidUtcIsoSeconds(recordedAt) {
		return nil, errors.New("Reconciliation timestamp must be a valid UTC ISO-8601 string.")
	}
	if recordedAt > nowUtc {
		return nil, errors.New("Reconciliation timestamp cannot be in the future.")
	}

	var rec db.AccountReconciliation
	var summary AccountSummary
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Account
		if err := tx.Where("id = ?", input.AccountID).First(&current).Error; err != nil {
			return errors.New("Account not found.")
		}

		balBefore, err := computeAccountBalanceAsOf(tx, current, "", recordedAt, nowUtc)
		if err != nil {
			return err
		}
		computedMinor := input.ActualBalanceMinor
		if balBefore.ComputedBalanceMinor != nil {
			computedMinor = *balBefore.ComputedBalanceMinor
		}
		adjMinor := input.ActualBalanceMinor - computedMinor

		var note *string
		if input.Note != nil && strings.TrimSpace(*input.Note) != "" {
			n := strings.TrimSpace(*input.Note)
			note = &n
		}

		rec = db.AccountReconciliation{
			ID:                   uuid.NewString(),
			AccountID:            input.AccountID,
			Currency:             balBefore.Currency,
			ComputedBalanceMinor: computedMinor,
			ActualBalanceMinor:   input.ActualBalanceMinor,
			AdjustmentMinor:      adjMinor,
			Note:                 note,
			RecordedAtUtc:        recordedAt,
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}

		if current.Status != "confirmed" {
			if err := tx.Model(&db.Account{}).Where("id = ?", input.AccountID).Update("status", "confirmed").Error; err != nil {
				return err
			}
			current.Status = "confirmed"
		}

		built, err := buildAccountSummaryAt(tx, current, "", "", nil, now)
		if err != nil {
			return err
		}
		summary = built
		summary.TypeLocked = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"reconciliation": rec,
		"account":        summary,
	}, nil
}

func ListAccountReconciliations(gdb *gorm.DB, accountID string) ([]db.AccountReconciliation, error) {
	rows := []db.AccountReconciliation{}
	if err := gdb.Where("account_id = ?", accountID).Order("recorded_at_utc DESC, rowid DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
