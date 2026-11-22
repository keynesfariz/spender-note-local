package domain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/llm"
)

var strictEmailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type AppSettingsView struct {
	CountryCode     *string             `json:"country_code"`
	Timezone        *string             `json:"timezone"`
	Currency        *string             `json:"currency"`
	LlmProvider     string              `json:"llm_provider"`
	LlmBaseURL      string              `json:"llm_base_url"`
	LlmModel        *string             `json:"llm_model"`
	LlmTimeoutMs    int                 `json:"llm_timeout_ms"`
	SyncBatchLimit  int                 `json:"sync_batch_limit"`
	CurrencyLocked  bool                `json:"currencyLocked"`
	AvailableModels []string            `json:"availableModels"`
	ServerReachable bool                `json:"serverReachable"`
	ServerError     *string             `json:"serverError"`
	CountryCatalog  []CountryLocaleInfo `json:"countryCatalog"`
}

type GmailStatusView struct {
	Status          string  `json:"status"`
	EmailAddress    *string `json:"email_address"`
	ExpiresAtUtc    *string `json:"expires_at_utc"`
	Scope           *string `json:"scope"`
	OAuthConfigured bool    `json:"oauthConfigured"`
}

func IsCurrencyLocked(gdb *gorm.DB) bool {
	var acctCnt int64
	if err := gdb.Model(&db.Account{}).Count(&acctCnt).Error; err != nil {
		return false
	}
	if acctCnt > 0 {
		return true
	}
	var txCnt int64
	if err := gdb.Model(&db.Transaction{}).Count(&txCnt).Error; err != nil {
		return false
	}
	return txCnt > 0
}

func GetAppSettings(ctx context.Context, gdb *gorm.DB, probeBaseURLOverride string) (*AppSettingsView, error) {
	var row db.AppSetting
	if err := gdb.First(&row, 1).Error; err != nil {
		return nil, errors.New("Application settings row is missing")
	}

	savedBaseURL := strings.TrimSpace(row.LlmBaseURL)
	if savedBaseURL == "" {
		savedBaseURL = db.DefaultLlmBaseURL
	}

	probeTarget := savedBaseURL
	if strings.TrimSpace(probeBaseURLOverride) != "" {
		probeTarget = strings.TrimSpace(probeBaseURLOverride)
	}

	availableModels := []string{}
	serverReachable := false
	var serverError *string

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	client, err := llm.NewClient(probeTarget, 2*time.Second)
	if err != nil {
		msg := err.Error()
		serverError = &msg
	} else {
		models, listErr := client.ListModels(probeCtx)
		if listErr != nil {
			msg := listErr.Error()
			serverError = &msg
		} else {
			availableModels = models
			serverReachable = true
		}
	}

	return &AppSettingsView{
		CountryCode:     row.CountryCode,
		Timezone:        row.Timezone,
		Currency:        row.Currency,
		LlmProvider:     row.LlmProvider,
		LlmBaseURL:      savedBaseURL,
		LlmModel:        row.LlmModel,
		LlmTimeoutMs:    row.LlmTimeoutMs,
		SyncBatchLimit:  row.SyncBatchLimit,
		CurrencyLocked:  IsCurrencyLocked(gdb),
		AvailableModels: availableModels,
		ServerReachable: serverReachable,
		ServerError:     serverError,
		CountryCatalog:  ListCountryCatalog(),
	}, nil
}

type UpdateLocaleInput struct {
	CountryCode string  `json:"country_code"`
	Timezone    *string `json:"timezone"`
	Currency    *string `json:"currency"`
}

func UpdateLocaleSettings(gdb *gorm.DB, input UpdateLocaleInput) (map[string]string, error) {
	country := FindCountryByCode(input.CountryCode)
	if country == nil {
		return nil, fmt.Errorf("Unsupported country code: %s", input.CountryCode)
	}

	tz := country.DefaultTimezone
	if input.Timezone != nil && strings.TrimSpace(*input.Timezone) != "" {
		tz = strings.TrimSpace(*input.Timezone)
	}
	if _, err := LoadTimezone(tz); err != nil {
		return nil, err
	}

	rawCurrency := country.Currency
	if input.Currency != nil && strings.TrimSpace(*input.Currency) != "" {
		rawCurrency = *input.Currency
	}
	currency, err := NormalizeCurrencyCode(rawCurrency)
	if err != nil {
		return nil, err
	}

	err = gdb.Transaction(func(tx *gorm.DB) error {
		var current db.AppSetting
		if err := tx.First(&current, 1).Error; err != nil {
			return err
		}
		if current.Currency != nil && *current.Currency != currency && IsCurrencyLocked(tx) {
			return errors.New("Currency cannot be changed once accounts or transactions exist.")
		}
		return tx.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
			"country_code": country.Code,
			"timezone":     tz,
			"currency":     currency,
		}).Error
	})
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"country_code": country.Code,
		"timezone":     tz,
		"currency":     currency,
	}, nil
}

type UpdateLlmInput struct {
	LlmBaseURL   *string `json:"llm_base_url"`
	LlmModel     *string `json:"llm_model"`
	LlmTimeoutMs *int    `json:"llm_timeout_ms"`
}

func UpdateLlmSettings(gdb *gorm.DB, input UpdateLlmInput) (map[string]any, error) {
	rawBaseURL := ""
	if input.LlmBaseURL != nil {
		rawBaseURL = *input.LlmBaseURL
	}
	normalizedBaseURL, err := llm.NormalizeBaseURL(rawBaseURL)
	if err != nil {
		return nil, err
	}

	var normalizedModel *string
	if input.LlmModel != nil {
		trimmed := strings.TrimSpace(*input.LlmModel)
		if trimmed != "" {
			normalizedModel = &trimmed
		}
	}

	timeoutMs := 60000
	if input.LlmTimeoutMs != nil {
		timeoutMs = *input.LlmTimeoutMs
	}
	if timeoutMs < 1000 || timeoutMs > 600000 {
		return nil, errors.New("Timeout must be an integer between 1000 and 600000 ms.")
	}

	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Updates(map[string]any{
		"llm_provider":   "openai_compatible",
		"llm_base_url":   normalizedBaseURL,
		"llm_model":      normalizedModel,
		"llm_timeout_ms": timeoutMs,
	}).Error; err != nil {
		return nil, err
	}

	return map[string]any{
		"llm_provider":   "openai_compatible",
		"llm_base_url":   normalizedBaseURL,
		"llm_model":      normalizedModel,
		"llm_timeout_ms": timeoutMs,
	}, nil
}

func UpdateSyncSettings(gdb *gorm.DB, syncBatchLimit int) (map[string]int, error) {
	if syncBatchLimit < 1 || syncBatchLimit > 500 {
		return nil, errors.New("Sync batch limit must be an integer between 1 and 500.")
	}
	if err := gdb.Model(&db.AppSetting{}).Where("id = 1").Update("sync_batch_limit", syncBatchLimit).Error; err != nil {
		return nil, err
	}
	return map[string]int{"sync_batch_limit": syncBatchLimit}, nil
}

func isValidRoleHint(role string) bool {
	return role == "auto" || role == "card_charge" || role == "receipt"
}

func ListSenders(gdb *gorm.DB) ([]db.Sender, error) {
	senders := []db.Sender{}
	if err := gdb.Order("email ASC").Find(&senders).Error; err != nil {
		return nil, err
	}
	return senders, nil
}

type CreateSenderInput struct {
	Email            string  `json:"email"`
	RoleHint         *string `json:"role_hint"`
	DefaultAccountID *string `json:"default_account_id"`
	Enabled          *bool   `json:"enabled"`
	BackfillFromUtc  *string `json:"backfill_from_utc"`
}

func CreateSender(gdb *gorm.DB, input CreateSenderInput) (*db.Sender, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if !strictEmailRe.MatchString(email) {
		return nil, errors.New("Enter a valid sender email address.")
	}

	roleHint := "auto"
	if input.RoleHint != nil && *input.RoleHint != "" {
		roleHint = *input.RoleHint
	}
	if !isValidRoleHint(roleHint) {
		return nil, fmt.Errorf("Invalid sender role hint: %s", roleHint)
	}

	var settings db.AppSetting
	if err := gdb.First(&settings, 1).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	tz := "UTC"
	if settings.Timezone != nil && *settings.Timezone != "" {
		tz = *settings.Timezone
	}

	backfillFromUtc := ""
	if input.BackfillFromUtc != nil && strings.TrimSpace(*input.BackfillFromUtc) != "" {
		backfillFromUtc = strings.TrimSpace(*input.BackfillFromUtc)
	} else {
		var err error
		backfillFromUtc, err = GetMonthStartUtc(GetCurrentMonthInTimezone(tz), tz)
		if err != nil {
			return nil, err
		}
	}
	if !IsValidUtcIsoSeconds(backfillFromUtc) {
		return nil, errors.New("Backfill date must be a valid UTC ISO-8601 timestamp.")
	}

	var defaultAccountID *string
	if input.DefaultAccountID != nil && strings.TrimSpace(*input.DefaultAccountID) != "" {
		trimmed := strings.TrimSpace(*input.DefaultAccountID)
		if err := AssertAccountExists(gdb, trimmed, "Selected default account does not exist."); err != nil {
			return nil, err
		}
		defaultAccountID = &trimmed
	}

	var dupCount int64
	if err := gdb.Model(&db.Sender{}).Where("email = ?", email).Count(&dupCount).Error; err != nil {
		return nil, err
	}
	if dupCount > 0 {
		return nil, fmt.Errorf("Sender %s is already configured.", email)
	}

	enabled := 1
	if input.Enabled != nil && !*input.Enabled {
		enabled = 0
	}

	sender := db.Sender{
		ID:               uuid.NewString(),
		Email:            email,
		RoleHint:         roleHint,
		DefaultAccountID: defaultAccountID,
		Enabled:          enabled,
		BackfillFromUtc:  backfillFromUtc,
	}
	if err := gdb.Create(&sender).Error; err != nil {
		return nil, err
	}
	return &sender, nil
}

type UpdateSenderInput struct {
	Email               *string `json:"email"`
	RoleHint            *string `json:"role_hint"`
	HasDefaultAccountID bool    `json:"-"`
	DefaultAccountID    *string `json:"default_account_id"`
	Enabled             *bool   `json:"enabled"`
	BackfillFromUtc     *string `json:"backfill_from_utc"`
}

func UpdateSender(gdb *gorm.DB, id string, patch UpdateSenderInput) (*db.Sender, error) {
	var updated db.Sender
	err := gdb.Transaction(func(tx *gorm.DB) error {
		var current db.Sender
		if err := tx.Where("id = ?", id).First(&current).Error; err != nil {
			return errors.New("Sender not found.")
		}

		email := current.Email
		if patch.Email != nil {
			email = strings.ToLower(strings.TrimSpace(*patch.Email))
		}
		if !strictEmailRe.MatchString(email) {
			return errors.New("Enter a valid sender email address.")
		}
		if email != current.Email {
			var dupCnt int64
			if err := tx.Model(&db.Sender{}).Where("email = ? AND id != ?", email, id).Count(&dupCnt).Error; err != nil {
				return err
			}
			if dupCnt > 0 {
				return fmt.Errorf("Sender %s is already configured.", email)
			}
		}

		roleHint := current.RoleHint
		if patch.RoleHint != nil {
			roleHint = *patch.RoleHint
		}
		if !isValidRoleHint(roleHint) {
			return fmt.Errorf("Invalid sender role hint: %s", roleHint)
		}

		defaultAccountID := current.DefaultAccountID
		if patch.HasDefaultAccountID {
			if patch.DefaultAccountID != nil && strings.TrimSpace(*patch.DefaultAccountID) != "" {
				trimmed := strings.TrimSpace(*patch.DefaultAccountID)
				if err := AssertAccountExists(tx, trimmed, "Selected default account does not exist."); err != nil {
					return err
				}
				defaultAccountID = &trimmed
			} else {
				defaultAccountID = nil
			}
		}

		enabled := current.Enabled
		if patch.Enabled != nil {
			if *patch.Enabled {
				enabled = 1
			} else {
				enabled = 0
			}
		}

		backfillFromUtc := current.BackfillFromUtc
		if patch.BackfillFromUtc != nil {
			backfillFromUtc = strings.TrimSpace(*patch.BackfillFromUtc)
		}
		if !IsValidUtcIsoSeconds(backfillFromUtc) {
			return errors.New("Backfill date must be a valid UTC ISO-8601 timestamp.")
		}

		newT, _ := time.Parse("2006-01-02T15:04:05Z", backfillFromUtc)
		curT, _ := time.Parse("2006-01-02T15:04:05Z", current.BackfillFromUtc)
		nextCursorMs := current.LastInternalDateMs
		if newT.Before(curT) {
			nextCursorMs = nil
		}

		if err := tx.Model(&db.Sender{}).Where("id = ?", id).Updates(map[string]any{
			"email":                 email,
			"role_hint":             roleHint,
			"default_account_id":    defaultAccountID,
			"enabled":               enabled,
			"backfill_from_utc":     backfillFromUtc,
			"last_internal_date_ms": nextCursorMs,
		}).Error; err != nil {
			return err
		}

		return tx.Where("id = ?", id).First(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func DeleteSender(gdb *gorm.DB, id string) error {
	res := gdb.Where("id = ?", id).Delete(&db.Sender{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("Sender not found.")
	}
	return nil
}

func GetGmailStatus(gdb *gorm.DB) (*GmailStatusView, error) {
	var row db.GmailOAuth
	err := gdb.First(&row, 1).Error
	status := "disconnected"
	var emailAddr, expiresAt, scope *string
	if err == nil {
		status = row.Status
		emailAddr = row.EmailAddress
		expiresAt = row.ExpiresAtUtc
		scope = row.Scope
	}
	configured := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")) != "" &&
		strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")) != ""
	return &GmailStatusView{
		Status:          status,
		EmailAddress:    emailAddr,
		ExpiresAtUtc:    expiresAt,
		Scope:           scope,
		OAuthConfigured: configured,
	}, nil
}
