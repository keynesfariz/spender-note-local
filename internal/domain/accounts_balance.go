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
}

func resolveAccountTimezone(gdb *gorm.DB, targetTimezone string) (string, error) {
	tz := strings.TrimSpace(targetTimezone)
	if tz == "" {
		var settings db.AppSetting
		err := gdb.Select("timezone").First(&settings, 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		if err == nil && settings.Timezone != nil {
			tz = strings.TrimSpace(*settings.Timezone)
		}
	}
	if tz == "" {
		tz = "UTC"
	}
	return tz, nil
}

func computeNetFlowAndBalance(accountType string, anchorBalance *int64, localTotal int64) (int64, *int64) {
	var netFlow int64
	if localTotal != 0 {
		if accountType == "credit_card" {
			netFlow = localTotal
		} else {
			netFlow = -localTotal
		}
	}
	var computed *int64
	if anchorBalance != nil {
		v := *anchorBalance + netFlow
		computed = &v
	}
	return netFlow, computed
}

func computeAccountBalanceAsOf(gdb *gorm.DB, account db.Account, asOfUtc, nowUtc string) (accountBalanceInfo, error) {
	var latestRec db.AccountReconciliation
	var latestRecPtr *db.AccountReconciliation
	err := gdb.Where("account_id = ? AND recorded_at_utc <= ?", account.ID, asOfUtc).
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
			account.Currency, account.Currency, account.ID, asOfUtc,
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
			account.Currency, account.Currency, account.ID, anchorTime, asOfUtc,
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
			account.Currency, account.Currency, account.ID, anchorTime, asOfUtc,
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

	netFlow, computed := computeNetFlowAndBalance(account.Type, anchorBalance, agg.LocalTotal)

	return accountBalanceInfo{
		ComputedBalanceMinor:   computed,
		NetFlowMinor:           netFlow,
		UncountedForeignCount:  agg.ForeignCnt,
		FutureTransactionCount: futureCnt,
		LatestReconciliation:   latestRecPtr,
	}, nil
}

func computeAccountBalance(gdb *gorm.DB, account db.Account) (accountBalanceInfo, error) {
	nowUtc := NowUtcIsoSeconds()
	return computeAccountBalanceAsOf(gdb, account, nowUtc, nowUtc)
}

func computeBillingCycleSummary(gdb *gorm.DB, account db.Account, tz string, now time.Time) (*CreditCardBillingCycleSummary, error) {
	if account.Type != "credit_card" || account.StatementDay == nil {
		return nil, nil
	}
	resolvedTz, err := resolveAccountTimezone(gdb, tz)
	if err != nil {
		return nil, err
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
		account.ID, account.Currency, bounds.CurrentCycle.StartUtc, bounds.CurrentCycle.EndExclusiveUtc, nowUtc,
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
		account.ID, account.Currency, bounds.LastCycle.StartUtc, bounds.LastCycle.EndExclusiveUtc,
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

func buildAccountSummaryAt(gdb *gorm.DB, account db.Account, targetTimezone string, identifiers []db.AccountIdentifier, now time.Time) (AccountSummary, error) {
	nowUtc := ToUtcIsoSeconds(now)
	bal, err := computeAccountBalanceAsOf(gdb, account, nowUtc, nowUtc)
	if err != nil {
		return AccountSummary{}, err
	}
	if identifiers == nil {
		identifiers, err = listIdentifiersForAccount(gdb, account.ID)
		if err != nil {
			return AccountSummary{}, err
		}
	}
	billingCycle, err := computeBillingCycleSummary(gdb, account, targetTimezone, now)
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

func buildAccountSummary(gdb *gorm.DB, account db.Account, identifiers []db.AccountIdentifier) (AccountSummary, error) {
	return buildAccountSummaryAt(gdb, account, "", identifiers, time.Now().UTC())
}

func buildAccountSummariesAt(
	gdb *gorm.DB,
	accounts []db.Account,
	identsByAcct map[string][]db.AccountIdentifier,
	targetTimezone string,
	now time.Time,
) ([]AccountSummary, error) {
	nowUtc := ToUtcIsoSeconds(now)

	// 1. Latest reconciliation per account (<= nowUtc) in a single query.
	var latestRecRows []db.AccountReconciliation
	if err := gdb.Raw(`
		SELECT id, account_id, currency, computed_balance_minor, actual_balance_minor, adjustment_minor, note, recorded_at_utc
		FROM (
			SELECT *,
				ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY recorded_at_utc DESC, rowid DESC) AS rn
			FROM account_reconciliations
			WHERE recorded_at_utc <= ?
		)
		WHERE rn = 1`, nowUtc).Scan(&latestRecRows).Error; err != nil {
		return nil, err
	}
	latestRecByAcct := make(map[string]*db.AccountReconciliation, len(latestRecRows))
	for i := range latestRecRows {
		rec := latestRecRows[i]
		latestRecByAcct[rec.AccountID] = &rec
	}

	// 2. Single scan of account-linked transactions to compute balances, future counts,
	//    type-lock status, and credit card billing-cycle spend for all accounts at once.
	type acctTxRow struct {
		AccountID     string `gorm:"column:account_id"`
		Currency      string `gorm:"column:currency"`
		AmountMinor   int64  `gorm:"column:amount_minor"`
		OccurredAtUtc string `gorm:"column:occurred_at_utc"`
	}
	var allTx []acctTxRow
	if err := gdb.Raw(`
		SELECT account_id, currency, amount_minor, occurred_at_utc
		FROM transactions
		WHERE account_id IS NOT NULL`).Scan(&allTx).Error; err != nil {
		return nil, err
	}

	type acctState struct {
		account           db.Account
		latestRec         *db.AccountReconciliation
		anchorBalance     *int64
		anchorTime        string
		strictAfterAnchor bool
		localTotal        int64
		foreignCnt        int64
		futureCnt         int64
		hasTx             bool
		cycleBounds       *CreditCardCycleBounds
		curCycleSpent     int64
		curCycleCount     int64
		lastCycleSpent    int64
		lastCycleCount    int64
	}

	states := make([]*acctState, len(accounts))
	stateByID := make(map[string]*acctState, len(accounts))
	for i, acct := range accounts {
		st := &acctState{
			account:   acct,
			latestRec: latestRecByAcct[acct.ID],
		}
		if st.latestRec != nil {
			b := st.latestRec.ActualBalanceMinor
			st.anchorBalance = &b
			st.anchorTime = st.latestRec.RecordedAtUtc
			st.strictAfterAnchor = true
		} else if acct.InitialBalanceMinor != nil && acct.InitialBalanceAsOfUtc <= nowUtc {
			st.anchorBalance = acct.InitialBalanceMinor
			st.anchorTime = acct.InitialBalanceAsOfUtc
		}
		if acct.Type == "credit_card" && acct.StatementDay != nil {
			if bounds, err := ComputeCreditCardCycleBounds(*acct.StatementDay, targetTimezone, now); err == nil {
				st.cycleBounds = &bounds
			}
		}
		states[i] = st
		stateByID[acct.ID] = st
	}

	for _, tx := range allTx {
		st, ok := stateByID[tx.AccountID]
		if !ok {
			continue
		}
		st.hasTx = true
		if tx.OccurredAtUtc > nowUtc {
			st.futureCnt++
		} else {
			inAnchorWindow := false
			if st.anchorBalance == nil {
				inAnchorWindow = true
			} else if st.strictAfterAnchor {
				inAnchorWindow = tx.OccurredAtUtc > st.anchorTime
			} else {
				inAnchorWindow = tx.OccurredAtUtc >= st.anchorTime
			}
			if inAnchorWindow {
				if tx.Currency == st.account.Currency {
					st.localTotal += tx.AmountMinor
				} else {
					st.foreignCnt++
				}
			}
		}

		if st.cycleBounds != nil && tx.Currency == st.account.Currency {
			cb := st.cycleBounds
			if tx.OccurredAtUtc >= cb.CurrentCycle.StartUtc && tx.OccurredAtUtc < cb.CurrentCycle.EndExclusiveUtc && tx.OccurredAtUtc <= nowUtc {
				st.curCycleSpent += tx.AmountMinor
				st.curCycleCount++
			}
			if tx.OccurredAtUtc >= cb.LastCycle.StartUtc && tx.OccurredAtUtc < cb.LastCycle.EndExclusiveUtc {
				st.lastCycleSpent += tx.AmountMinor
				st.lastCycleCount++
			}
		}
	}

	out := make([]AccountSummary, 0, len(accounts))
	for _, st := range states {
		idents := identsByAcct[st.account.ID]
		if idents == nil {
			idents = []db.AccountIdentifier{}
		}

		netFlow, computed := computeNetFlowAndBalance(st.account.Type, st.anchorBalance, st.localTotal)

		var billingCycle *CreditCardBillingCycleSummary
		if st.cycleBounds != nil && st.account.StatementDay != nil {
			billingCycle = &CreditCardBillingCycleSummary{
				StatementDay:      *st.account.StatementDay,
				LastStatementDate: st.cycleBounds.LastStatementDate,
				CurrentCycle: BillingCycleSpendSummary{
					StartDate:        st.cycleBounds.CurrentCycle.StartDate,
					EndDate:          st.cycleBounds.CurrentCycle.EndDate,
					SpentMinor:       st.curCycleSpent,
					TransactionCount: st.curCycleCount,
				},
				LastCycle: BillingCycleSpendSummary{
					StartDate:        st.cycleBounds.LastCycle.StartDate,
					EndDate:          st.cycleBounds.LastCycle.EndDate,
					SpentMinor:       st.lastCycleSpent,
					TransactionCount: st.lastCycleCount,
				},
			}
		}

		typeLocked := st.latestRec != nil || (st.account.Status != "provisional" && st.hasTx)

		out = append(out, AccountSummary{
			Account:                st.account,
			Identifiers:            idents,
			ComputedBalanceMinor:   computed,
			NetFlowMinor:           netFlow,
			UncountedForeignCount:  st.foreignCnt,
			FutureTransactionCount: st.futureCnt,
			LatestReconciliation:   st.latestRec,
			BillingCycle:           billingCycle,
			TypeLocked:             typeLocked,
		})
	}
	return out, nil
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

		balBefore, err := computeAccountBalanceAsOf(tx, current, recordedAt, nowUtc)
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
			Currency:             current.Currency,
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

		built, err := buildAccountSummaryAt(tx, current, "", nil, now)
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
