package domain

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
)

const (
	DedupWindowMs               int64 = 10 * 60 * 1000
	merchantSimilarityThreshold       = 0.5
	maxStackedPrefixPasses            = 2
)

var (
	processorPrefixRe = regexp.MustCompile(`(?i)^(?:sq\s*\*|tst\s*\*|paypal\s*\*|pp\s*\*|apl\s*\*|apple\s*pay\s*\*|amzn\s*mktp|amzn\s*\*|stripe\s*\*|sp\s*\*|goog\s*\*|pos\s*\*)\s*`)
	storeIDRe         = regexp.MustCompile(`(?i)(?:#\s*\d+|\*\s*[a-z0-9]{3,}|\b(?:store|loc)\s*#?\s*\d+\b)`)
	legalSuffixRe     = regexp.MustCompile(`(?i)\b(?:inc|llc|ltd|corp|co|plc|pt|tbk|sdn\s+bhd|pte|gmbh|s\.a\.|sa)\b\.?`)
	nonAlnumRe        = regexp.MustCompile(`[^a-z0-9]+`)
)

type ExtractedTransactionInput struct {
	EmailID           string
	EmailKind         string
	AmountMinor       int64
	Currency          string
	OccurredAtUtc     string
	MerchantRaw       string
	MerchantNorm      string
	Category          string
	CardLast4         *string
	ReferenceCode     *string
	ResolvedAccountID *string
	NowUtc            string
}

type DeduplicationResult struct {
	Outcome       string
	TransactionID string
	SuggestionIDs []string
}

func NormalizeMerchant(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	for i := 0; i < maxStackedPrefixPasses; i++ {
		s = processorPrefixRe.ReplaceAllString(s, "")
	}
	s = storeIDRe.ReplaceAllString(s, " ")
	s = legalSuffixRe.ReplaceAllString(s, " ")
	s = nonAlnumRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(multiSpaceRe.ReplaceAllString(s, " "))
}

func computeNormalizedSimilarity(na, nb string) float64 {
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1.0
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		minLen := float64(min(len(na), len(nb)))
		maxLen := float64(max(len(na), len(nb)))
		return math.Max(0.7, minLen/maxLen)
	}
	tokA := map[string]struct{}{}
	for _, t := range strings.Fields(na) {
		tokA[t] = struct{}{}
	}
	tokB := map[string]struct{}{}
	for _, t := range strings.Fields(nb) {
		tokB[t] = struct{}{}
	}
	if len(tokA) == 0 || len(tokB) == 0 {
		return 0
	}
	inter := 0
	for t := range tokA {
		if _, ok := tokB[t]; ok {
			inter++
		}
	}
	union := len(tokA) + len(tokB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func canonicalPair(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func loadRejectedEmailIDs(tx *gorm.DB, emailID string) map[string]struct{} {
	var rows []db.RejectedMerge
	tx.Where("email_a_id = ? OR email_b_id = ?", emailID, emailID).Find(&rows)
	out := map[string]struct{}{}
	for _, r := range rows {
		if r.EmailAID == emailID {
			out[r.EmailBID] = struct{}{}
		} else {
			out[r.EmailAID] = struct{}{}
		}
	}
	return out
}

func insertRejectedPair(tx *gorm.DB, emailA, emailB, nowUtc string) {
	if emailA == emailB {
		return
	}
	first, second := canonicalPair(emailA, emailB)
	tx.Exec(`INSERT OR IGNORE INTO rejected_merges (email_a_id, email_b_id, created_at_utc) VALUES (?, ?, ?)`, first, second, nowUtc)
}

type candidateSourceRow struct {
	EmailID       string  `gorm:"column:email_id"`
	Role          string  `gorm:"column:role"`
	MerchantRaw   *string `gorm:"column:merchant_raw"`
	CardLast4     *string `gorm:"column:card_last4"`
	ReferenceCode *string `gorm:"column:reference_code"`
}

func RecomputeTransactionFromSources(tx *gorm.DB, transactionID string, nowUtc string) {
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	var t db.Transaction
	if err := tx.Select("id, source_type, manual_override").Where("id = ?", transactionID).First(&t).Error; err != nil {
		return
	}
	if t.SourceType == "manual" {
		return
	}
	if t.ManualOverride == 1 {
		tx.Model(&db.Transaction{}).Where("id = ?", transactionID).Update("updated_at_utc", nowUtc)
		return
	}

	type sourceExt struct {
		EmailID          string  `gorm:"column:email_id"`
		Role             string  `gorm:"column:role"`
		AmountMinor      *int64  `gorm:"column:amount_minor"`
		Currency         *string `gorm:"column:currency"`
		OccurredAtUtc    *string `gorm:"column:occurred_at_utc"`
		MerchantRaw      *string `gorm:"column:merchant_raw"`
		Category         *string `gorm:"column:category"`
		CardLast4        *string `gorm:"column:card_last4"`
		BankAccountHint  *string `gorm:"column:bank_account_hint"`
		PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
		LlmAccountID     *string `gorm:"column:llm_account_id"`
		SenderID         *string `gorm:"column:sender_id"`
	}
	var sources []sourceExt
	tx.Raw(`
		SELECT
			ts.email_id, ts.role,
			ee.amount_minor, ee.currency, ee.occurred_at_utc,
			ee.merchant_raw, ee.category, ee.card_last4,
			ee.bank_account_hint, ee.payment_method_raw, ee.llm_account_id,
			re.sender_id
		FROM transaction_sources ts
		JOIN raw_emails re ON re.id = ts.email_id
		LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
		WHERE ts.transaction_id = ?
		ORDER BY re.internal_date_ms ASC, ts.email_id ASC`, transactionID).Scan(&sources)
	if len(sources) == 0 {
		return
	}

	var cardCharge, receipt *sourceExt
	for i := range sources {
		if sources[i].Role == "card_charge" && cardCharge == nil {
			cardCharge = &sources[i]
		}
		if sources[i].Role == "receipt" && receipt == nil {
			receipt = &sources[i]
		}
	}
	primary := &sources[0]
	if cardCharge != nil {
		primary = cardCharge
	}

	merchant := "Unknown Merchant"
	if receipt != nil && receipt.MerchantRaw != nil && *receipt.MerchantRaw != "" {
		merchant = *receipt.MerchantRaw
	} else if primary.MerchantRaw != nil && *primary.MerchantRaw != "" {
		merchant = *primary.MerchantRaw
	}

	category := DefaultCategory
	if receipt != nil && receipt.Category != nil && *receipt.Category != "" {
		category = *receipt.Category
	} else if primary.Category != nil && *primary.Category != "" {
		category = *primary.Category
	}

	var accountID *string
	if cardCharge != nil {
		accountID = MatchAccountForExtraction(tx, MatchAccountParams{
			SenderID:        cardCharge.SenderID,
			CardLast4:       cardCharge.CardLast4,
			BankAccountHint: cardCharge.BankAccountHint,
			PaymentMethod:   cardCharge.PaymentMethodRaw,
			LlmAccountID:    cardCharge.LlmAccountID,
		})
	}
	if accountID == nil {
		for i := range sources {
			if cardCharge != nil && &sources[i] == cardCharge {
				continue
			}
			accountID = MatchAccountForExtraction(tx, MatchAccountParams{
				SenderID:        sources[i].SenderID,
				CardLast4:       sources[i].CardLast4,
				BankAccountHint: sources[i].BankAccountHint,
				PaymentMethod:   sources[i].PaymentMethodRaw,
				LlmAccountID:    sources[i].LlmAccountID,
			})
			if accountID != nil {
				break
			}
		}
	}

	if primary.AmountMinor != nil && primary.Currency != nil && primary.OccurredAtUtc != nil {
		tx.Exec(`
			UPDATE transactions
			SET account_id = COALESCE(?, account_id),
				amount_minor = ?,
				currency = ?,
				occurred_at_utc = ?,
				merchant = ?,
				category = ?,
				updated_at_utc = ?
			WHERE id = ?`,
			accountID, *primary.AmountMinor, *primary.Currency, *primary.OccurredAtUtc,
			merchant, category, nowUtc, transactionID,
		)
	}
}

func scoreComplementaryCandidate(
	input ExtractedTransactionInput,
	inputMs int64,
	inputMerchantNorm string,
	cand db.Transaction,
	candSources []candidateSourceRow,
	rejectedEmailIDs map[string]struct{},
) (deltaMs int64, similarity float64, ok bool) {
	for _, s := range candSources {
		if s.Role == input.EmailKind {
			return 0, 0, false
		}
		if _, rejected := rejectedEmailIDs[s.EmailID]; rejected {
			return 0, 0, false
		}
	}
	candMs, err := UtcIsoToEpochMs(cand.OccurredAtUtc)
	if err != nil {
		return 0, 0, false
	}
	diff := candMs - inputMs
	if diff < 0 {
		diff = -diff
	}
	if diff > DedupWindowMs {
		return 0, 0, false
	}

	merchantSim := computeNormalizedSimilarity(inputMerchantNorm, NormalizeMerchant(cand.Merchant))
	for _, s := range candSources {
		if s.MerchantRaw != nil {
			sim := computeNormalizedSimilarity(inputMerchantNorm, NormalizeMerchant(*s.MerchantRaw))
			if sim > merchantSim {
				merchantSim = sim
			}
		}
	}

	cardMatch := false
	if input.CardLast4 != nil && *input.CardLast4 != "" {
		for _, s := range candSources {
			if s.CardLast4 != nil && *s.CardLast4 == *input.CardLast4 {
				cardMatch = true
				break
			}
		}
	}

	if merchantSim < merchantSimilarityThreshold && !cardMatch {
		return 0, 0, false
	}
	score := merchantSim
	if cardMatch && score < 0.6 {
		score = 0.6
	}
	return diff, score, true
}

func DeduplicateAndSaveTransaction(gdb *gorm.DB, input ExtractedTransactionInput) (*DeduplicationResult, error) {
	nowUtc := input.NowUtc
	if nowUtc == "" {
		nowUtc = NowUtcIsoSeconds()
	}
	inputMerchantNorm := NormalizeMerchant(input.MerchantRaw)
	merchantNorm := input.MerchantNorm
	if merchantNorm == "" {
		merchantNorm = inputMerchantNorm
	}
	inputMs, err := UtcIsoToEpochMs(input.OccurredAtUtc)
	if err != nil {
		return nil, err
	}
	windowStart := ToUtcIsoSeconds(time.UnixMilli(inputMs - DedupWindowMs))
	windowEnd := ToUtcIsoSeconds(time.UnixMilli(inputMs + DedupWindowMs))

	var result DeduplicationResult
	err = gdb.Transaction(func(tx *gorm.DB) error {
		tx.Exec(`
			INSERT OR IGNORE INTO email_extractions (
				id, email_id, email_kind, amount_minor, currency, occurred_at_utc,
				merchant_raw, merchant_norm, category, card_last4, reference_code,
				extraction_model, raw_llm_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'dedup', '{}')`,
			uuid.NewString(), input.EmailID, input.EmailKind, input.AmountMinor, input.Currency,
			input.OccurredAtUtc, input.MerchantRaw, merchantNorm, input.Category, input.CardLast4, input.ReferenceCode,
		)

		resolvedAccount := input.ResolvedAccountID
		if resolvedAccount == nil {
			resolvedAccount = MatchAccountForExtraction(tx, MatchAccountParams{CardLast4: input.CardLast4})
		}
		rejectedIDs := loadRejectedEmailIDs(tx, input.EmailID)

		var candidates []db.Transaction
		tx.Where("source_type = 'gmail' AND occurred_at_utc >= ? AND occurred_at_utc <= ?", windowStart, windowEnd).Find(&candidates)

		sourcesCache := map[string][]candidateSourceRow{}
		getSources := func(txID string) []candidateSourceRow {
			if cached, ok := sourcesCache[txID]; ok {
				return cached
			}
			var rows []candidateSourceRow
			tx.Raw(`
				SELECT ts.email_id, ts.role, ee.merchant_raw, ee.card_last4, ee.reference_code
				FROM transaction_sources ts
				LEFT JOIN email_extractions ee ON ee.email_id = ts.email_id
				WHERE ts.transaction_id = ?`, txID).Scan(&rows)
			sourcesCache[txID] = rows
			return rows
		}

		// 1. Twin card charge by reference code
		if input.EmailKind == "card_charge" && input.ReferenceCode != nil && *input.ReferenceCode != "" {
			for _, cand := range candidates {
				if cand.AmountMinor != input.AmountMinor || cand.Currency != input.Currency {
					continue
				}
				cSources := getSources(cand.ID)
				rejected := false
				matchedRef := false
				for _, s := range cSources {
					if _, bad := rejectedIDs[s.EmailID]; bad {
						rejected = true
						break
					}
					if s.Role == "card_charge" && s.ReferenceCode != nil && *s.ReferenceCode == *input.ReferenceCode {
						matchedRef = true
					}
				}
				if rejected || !matchedRef {
					continue
				}
				tx.Create(&db.TransactionSource{
					ID:            uuid.NewString(),
					TransactionID: cand.ID,
					EmailID:       input.EmailID,
					Role:          input.EmailKind,
					MatchScore:    1.0,
					MatchState:    "auto",
				})
				if resolvedAccount != nil && cand.ManualOverride == 0 {
					tx.Exec(`UPDATE transactions SET account_id = COALESCE(account_id, ?) WHERE id = ?`, *resolvedAccount, cand.ID)
				}
				RecomputeTransactionFromSources(tx, cand.ID, nowUtc)
				result = DeduplicationResult{Outcome: "merged", TransactionID: cand.ID, SuggestionIDs: []string{}}
				return nil
			}
		}

		// 2. Complementary auto-merge
		type autoMatch struct {
			txID       string
			deltaMs    int64
			similarity float64
			cand       db.Transaction
		}
		var matches []autoMatch
		for _, cand := range candidates {
			if cand.AmountMinor != input.AmountMinor || cand.Currency != input.Currency {
				continue
			}
			delta, sim, ok := scoreComplementaryCandidate(input, inputMs, inputMerchantNorm, cand, getSources(cand.ID), rejectedIDs)
			if ok {
				matches = append(matches, autoMatch{cand.ID, delta, sim, cand})
			}
		}
		if len(matches) > 0 {
			sort.Slice(matches, func(i, j int) bool {
				if matches[i].deltaMs != matches[j].deltaMs {
					return matches[i].deltaMs < matches[j].deltaMs
				}
				if matches[i].similarity != matches[j].similarity {
					return matches[i].similarity > matches[j].similarity
				}
				return matches[i].txID < matches[j].txID
			})
			winner := matches[0]
			tx.Create(&db.TransactionSource{
				ID:            uuid.NewString(),
				TransactionID: winner.txID,
				EmailID:       input.EmailID,
				Role:          input.EmailKind,
				MatchScore:    winner.similarity,
				MatchState:    "auto",
			})
			if resolvedAccount != nil && winner.cand.ManualOverride == 0 {
				tx.Exec(`
					UPDATE transactions
					SET account_id = CASE WHEN ? = 'card_charge' THEN ? ELSE COALESCE(account_id, ?) END
					WHERE id = ?`, input.EmailKind, *resolvedAccount, *resolvedAccount, winner.txID)
			}
			RecomputeTransactionFromSources(tx, winner.txID, nowUtc)
			result = DeduplicationResult{Outcome: "merged", TransactionID: winner.txID, SuggestionIDs: []string{}}
			return nil
		}

		// 3. Create new transaction + near-match suggestions
		newTxID := uuid.NewString()
		if err := tx.Create(&db.Transaction{
			ID:             newTxID,
			SourceType:     "gmail",
			AccountID:      resolvedAccount,
			AmountMinor:    input.AmountMinor,
			Currency:       input.Currency,
			OccurredAtUtc:  input.OccurredAtUtc,
			Merchant:       input.MerchantRaw,
			Category:       input.Category,
			ManualOverride: 0,
			CreatedAtUtc:   nowUtc,
			UpdatedAtUtc:   nowUtc,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&db.TransactionSource{
			ID:            uuid.NewString(),
			TransactionID: newTxID,
			EmailID:       input.EmailID,
			Role:          input.EmailKind,
			MatchScore:    1.0,
			MatchState:    "single",
		}).Error; err != nil {
			return err
		}

		suggestionIDs := []string{}
		for _, cand := range candidates {
			if cand.AmountMinor == input.AmountMinor && cand.Currency == input.Currency {
				continue
			}
			_, sim, ok := scoreComplementaryCandidate(input, inputMs, inputMerchantNorm, cand, getSources(cand.ID), rejectedIDs)
			if !ok {
				continue
			}
			srcID, tgtID := canonicalPair(newTxID, cand.ID)
			sugID := uuid.NewString()
			res := tx.Exec(`
				INSERT OR IGNORE INTO merge_suggestions (
					id, source_transaction_id, target_transaction_id, score, status, created_at_utc
				) VALUES (?, ?, ?, ?, 'pending', ?)`, sugID, srcID, tgtID, sim, nowUtc)
			if res.RowsAffected > 0 {
				suggestionIDs = append(suggestionIDs, sugID)
			}
		}

		result = DeduplicationResult{Outcome: "created", TransactionID: newTxID, SuggestionIDs: suggestionIDs}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func RunPostMergeDedupForAccount(tx *gorm.DB, accountID, nowUtc string) []string {
	var rows []db.Transaction
	tx.Where("account_id = ?", accountID).Order("occurred_at_utc ASC, id ASC").Find(&rows)
	if len(rows) < 2 {
		return []string{}
	}

	type txMeta struct {
		tx   db.Transaction
		ms   int64
		norm string
	}
	txs := make([]txMeta, 0, len(rows))
	for _, r := range rows {
		ms, _ := UtcIsoToEpochMs(r.OccurredAtUtc)
		txs = append(txs, txMeta{tx: r, ms: ms, norm: NormalizeMerchant(r.Merchant)})
	}

	rejectedSet := map[string]struct{}{}
	var rejRows []db.RejectedMerge
	tx.Find(&rejRows)
	for _, r := range rejRows {
		rejectedSet[r.EmailAID+"::"+r.EmailBID] = struct{}{}
	}

	type srcPair struct {
		TransactionID string `gorm:"column:transaction_id"`
		EmailID       string `gorm:"column:email_id"`
	}
	var srcRows []srcPair
	tx.Raw(`
		SELECT ts.transaction_id, ts.email_id
		FROM transaction_sources ts
		JOIN transactions t ON t.id = ts.transaction_id
		WHERE t.account_id = ?`, accountID).Scan(&srcRows)
	emailsByTx := map[string][]string{}
	for _, s := range srcRows {
		emailsByTx[s.TransactionID] = append(emailsByTx[s.TransactionID], s.EmailID)
	}

	suggestionIDs := []string{}
	for i := 0; i < len(txs); i++ {
		a := txs[i]
		for j := i + 1; j < len(txs); j++ {
			b := txs[j]
			if b.ms-a.ms > DedupWindowMs {
				break
			}
			if a.tx.Currency != b.tx.Currency || a.tx.AmountMinor != b.tx.AmountMinor {
				continue
			}
			isRej := false
			for _, ea := range emailsByTx[a.tx.ID] {
				for _, eb := range emailsByTx[b.tx.ID] {
					f, s := canonicalPair(ea, eb)
					if _, ok := rejectedSet[f+"::"+s]; ok {
						isRej = true
						break
					}
				}
				if isRej {
					break
				}
			}
			if isRej {
				continue
			}

			sim := 0.7
			if a.norm == b.norm {
				sim = 1.0
			} else if a.norm != "" && b.norm != "" && (strings.Contains(a.norm, b.norm) || strings.Contains(b.norm, a.norm)) {
				sim = 0.85
			}
			srcID, tgtID := canonicalPair(a.tx.ID, b.tx.ID)
			sugID := uuid.NewString()
			res := tx.Exec(`
				INSERT OR IGNORE INTO merge_suggestions (
					id, source_transaction_id, target_transaction_id, score, status, created_at_utc
				) VALUES (?, ?, ?, ?, 'pending', ?)`, sugID, srcID, tgtID, sim, nowUtc)
			if res.RowsAffected > 0 {
				suggestionIDs = append(suggestionIDs, sugID)
			}
		}
	}
	return suggestionIDs
}

func ConfirmMergeSuggestion(gdb *gorm.DB, suggestionID string) (string, error) {
	nowUtc := NowUtcIsoSeconds()
	var survivorID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var sug db.MergeSuggestion
		if err := tx.Where("id = ?", suggestionID).First(&sug).Error; err != nil {
			return fmt.Errorf("Merge suggestion not found: %s", suggestionID)
		}
		if sug.Status != "pending" {
			return fmt.Errorf("Merge suggestion %s is not pending (%s)", suggestionID, sug.Status)
		}
		var txA, txB db.Transaction
		if err := tx.Where("id = ?", sug.SourceTransactionID).First(&txA).Error; err != nil {
			return errors.New("Cannot merge: one or both transactions no longer exist")
		}
		if err := tx.Where("id = ?", sug.TargetTransactionID).First(&txB).Error; err != nil {
			return errors.New("Cannot merge: one or both transactions no longer exist")
		}

		survivorID = txA.ID
		absorbedID := txB.ID
		if txB.ManualOverride == 1 && txA.ManualOverride == 0 {
			survivorID = txB.ID
			absorbedID = txA.ID
		}

		tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", absorbedID).Updates(map[string]any{
			"transaction_id": survivorID,
			"match_state":    "confirmed",
		})
		tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", survivorID).Update("match_state", "confirmed")
		tx.Model(&db.MergeSuggestion{}).Where("id = ?", suggestionID).Update("status", "accepted")
		tx.Where("id = ?", absorbedID).Delete(&db.Transaction{})

		RecomputeTransactionFromSources(tx, survivorID, nowUtc)
		return nil
	})
	return survivorID, err
}

func DismissMergeSuggestion(gdb *gorm.DB, suggestionID string) error {
	nowUtc := NowUtcIsoSeconds()
	return gdb.Transaction(func(tx *gorm.DB) error {
		var sug db.MergeSuggestion
		if err := tx.Where("id = ?", suggestionID).First(&sug).Error; err != nil {
			return fmt.Errorf("Merge suggestion not found: %s", suggestionID)
		}
		tx.Model(&db.MergeSuggestion{}).Where("id = ?", suggestionID).Update("status", "dismissed")

		var srcEmails, tgtEmails []string
		tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", sug.SourceTransactionID).Pluck("email_id", &srcEmails)
		tx.Model(&db.TransactionSource{}).Where("transaction_id = ?", sug.TargetTransactionID).Pluck("email_id", &tgtEmails)
		for _, s := range srcEmails {
			for _, t := range tgtEmails {
				insertRejectedPair(tx, s, t, nowUtc)
			}
		}
		return nil
	})
}

func SplitTransactionSource(gdb *gorm.DB, transactionID, emailID string) (string, error) {
	nowUtc := NowUtcIsoSeconds()
	var newTxID string
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var sources []db.TransactionSource
		tx.Where("transaction_id = ?", transactionID).Find(&sources)
		if len(sources) < 2 {
			return fmt.Errorf("Transaction %s must have at least 2 sources to split", transactionID)
		}
		var detached *db.TransactionSource
		var remaining []db.TransactionSource
		for i := range sources {
			if sources[i].EmailID == emailID {
				detached = &sources[i]
			} else {
				remaining = append(remaining, sources[i])
			}
		}
		if detached == nil {
			return fmt.Errorf("Email %s is not attached to transaction %s", emailID, transactionID)
		}

		type extInfo struct {
			AmountMinor      *int64  `gorm:"column:amount_minor"`
			Currency         *string `gorm:"column:currency"`
			OccurredAtUtc    *string `gorm:"column:occurred_at_utc"`
			MerchantRaw      *string `gorm:"column:merchant_raw"`
			Category         *string `gorm:"column:category"`
			CardLast4        *string `gorm:"column:card_last4"`
			BankAccountHint  *string `gorm:"column:bank_account_hint"`
			PaymentMethodRaw *string `gorm:"column:payment_method_raw"`
			LlmAccountID     *string `gorm:"column:llm_account_id"`
			SenderID         *string `gorm:"column:sender_id"`
		}
		var ext extInfo
		tx.Raw(`
			SELECT ee.amount_minor, ee.currency, ee.occurred_at_utc, ee.merchant_raw,
			       ee.category, ee.card_last4, ee.bank_account_hint, ee.payment_method_raw,
			       ee.llm_account_id, re.sender_id
			FROM raw_emails re
			LEFT JOIN email_extractions ee ON ee.email_id = re.id
			WHERE re.id = ?`, emailID).Scan(&ext)

		newTxID = uuid.NewString()
		acctID := MatchAccountForExtraction(tx, MatchAccountParams{
			SenderID:        ext.SenderID,
			CardLast4:       ext.CardLast4,
			BankAccountHint: ext.BankAccountHint,
			PaymentMethod:   ext.PaymentMethodRaw,
			LlmAccountID:    ext.LlmAccountID,
		})

		amountMinor := int64(1)
		if ext.AmountMinor != nil {
			amountMinor = *ext.AmountMinor
		}
		currency := "USD"
		if ext.Currency != nil && *ext.Currency != "" {
			currency = *ext.Currency
		}
		occurred := nowUtc
		if ext.OccurredAtUtc != nil && *ext.OccurredAtUtc != "" {
			occurred = *ext.OccurredAtUtc
		}
		merchant := "Unknown Merchant"
		if ext.MerchantRaw != nil && *ext.MerchantRaw != "" {
			merchant = *ext.MerchantRaw
		}
		category := DefaultCategory
		if ext.Category != nil && *ext.Category != "" {
			category = *ext.Category
		}

		if err := tx.Create(&db.Transaction{
			ID:             newTxID,
			SourceType:     "gmail",
			AccountID:      acctID,
			AmountMinor:    amountMinor,
			Currency:       currency,
			OccurredAtUtc:  occurred,
			Merchant:       merchant,
			Category:       category,
			ManualOverride: 0,
			CreatedAtUtc:   nowUtc,
			UpdatedAtUtc:   nowUtc,
		}).Error; err != nil {
			return err
		}

		tx.Model(&db.TransactionSource{}).Where("id = ?", detached.ID).Updates(map[string]any{
			"transaction_id": newTxID,
			"match_state":    "single",
			"match_score":    1.0,
		})

		for _, rem := range remaining {
			insertRejectedPair(tx, emailID, rem.EmailID, nowUtc)
		}
		if len(remaining) == 1 {
			tx.Model(&db.TransactionSource{}).Where("id = ?", remaining[0].ID).Updates(map[string]any{
				"match_state": "single",
				"match_score": 1.0,
			})
		}

		RecomputeTransactionFromSources(tx, transactionID, nowUtc)
		RecomputeTransactionFromSources(tx, newTxID, nowUtc)
		return nil
	})
	return newTxID, err
}
