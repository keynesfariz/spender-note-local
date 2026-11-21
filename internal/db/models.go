package db

type AppSetting struct {
	ID              int     `gorm:"column:id;primaryKey" json:"id"`
	CountryCode     *string `gorm:"column:country_code" json:"country_code"`
	Timezone        *string `gorm:"column:timezone" json:"timezone"`
	Currency        *string `gorm:"column:currency" json:"currency"`
	LlmProvider     string  `gorm:"column:llm_provider;not null;default:gguf" json:"llm_provider"`
	LlmModelPath    *string `gorm:"column:llm_model_path" json:"llm_model_path"`
	LlmContextSize  int     `gorm:"column:llm_context_size;not null;default:8192" json:"llm_context_size"`
	LlmTimeoutMs    int     `gorm:"column:llm_timeout_ms;not null;default:60000" json:"llm_timeout_ms"`
	SyncBatchLimit  int     `gorm:"column:sync_batch_limit;not null;default:50" json:"sync_batch_limit"`
}

func (AppSetting) TableName() string { return "app_settings" }

type GmailOAuth struct {
	ID           int     `gorm:"column:id;primaryKey" json:"id"`
	Status       string  `gorm:"column:status;not null;default:disconnected" json:"status"`
	EmailAddress *string `gorm:"column:email_address" json:"email_address"`
	AccessToken  *string `gorm:"column:access_token" json:"-"`
	RefreshToken *string `gorm:"column:refresh_token" json:"-"`
	ExpiresAtUtc *string `gorm:"column:expires_at_utc" json:"expires_at_utc"`
	Scope        *string `gorm:"column:scope" json:"scope"`
}

func (GmailOAuth) TableName() string { return "gmail_oauth" }

type OAuthState struct {
	State        string `gorm:"column:state;primaryKey"`
	CodeVerifier string `gorm:"column:code_verifier;not null"`
	ExpiresAtUtc string `gorm:"column:expires_at_utc;not null"`
}

func (OAuthState) TableName() string { return "oauth_states" }

type Account struct {
	ID                     string  `gorm:"column:id;primaryKey" json:"id"`
	Name                   string  `gorm:"column:name;not null" json:"name"`
	Type                   string  `gorm:"column:type;not null" json:"type"`
	Currency               string  `gorm:"column:currency;not null" json:"currency"`
	InitialBalanceMinor    *int64  `gorm:"column:initial_balance_minor" json:"initial_balance_minor"`
	InitialBalanceAsOfUtc  string  `gorm:"column:initial_balance_as_of_utc;not null" json:"initial_balance_as_of_utc"`
	Last4                  *string `gorm:"column:last4" json:"last4"`
	Status                 string  `gorm:"column:status;not null;default:confirmed" json:"status"`
	CreatedAtUtc           string  `gorm:"column:created_at_utc;not null" json:"created_at_utc"`
}

func (Account) TableName() string { return "accounts" }

type AccountIdentifier struct {
	ID              string `gorm:"column:id;primaryKey" json:"id"`
	AccountID       string `gorm:"column:account_id;not null" json:"account_id"`
	Kind            string `gorm:"column:kind;not null" json:"kind"`
	Value           string `gorm:"column:value;not null" json:"value"`
	NormalizedValue string `gorm:"column:normalized_value;not null" json:"normalized_value"`
	CreatedAtUtc    string `gorm:"column:created_at_utc;not null" json:"created_at_utc"`
}

func (AccountIdentifier) TableName() string { return "account_identifiers" }

type AccountReconciliation struct {
	ID                   string  `gorm:"column:id;primaryKey" json:"id"`
	AccountID            string  `gorm:"column:account_id;not null" json:"account_id"`
	Currency             string  `gorm:"column:currency;not null" json:"currency"`
	ComputedBalanceMinor int64   `gorm:"column:computed_balance_minor;not null" json:"computed_balance_minor"`
	ActualBalanceMinor   int64   `gorm:"column:actual_balance_minor;not null" json:"actual_balance_minor"`
	AdjustmentMinor      int64   `gorm:"column:adjustment_minor;not null" json:"adjustment_minor"`
	Note                 *string `gorm:"column:note" json:"note"`
	RecordedAtUtc        string  `gorm:"column:recorded_at_utc;not null" json:"recorded_at_utc"`
}

func (AccountReconciliation) TableName() string { return "account_reconciliations" }

type Sender struct {
	ID                 string  `gorm:"column:id;primaryKey" json:"id"`
	Email              string  `gorm:"column:email;not null;unique" json:"email"`
	RoleHint           string  `gorm:"column:role_hint;not null;default:auto" json:"role_hint"`
	DefaultAccountID   *string `gorm:"column:default_account_id" json:"default_account_id"`
	Enabled            int     `gorm:"column:enabled;not null;default:1" json:"enabled"`
	BackfillFromUtc    string  `gorm:"column:backfill_from_utc;not null" json:"backfill_from_utc"`
	LastInternalDateMs *int64  `gorm:"column:last_internal_date_ms" json:"last_internal_date_ms"`
	LastSyncAtUtc      *string `gorm:"column:last_sync_at_utc" json:"last_sync_at_utc"`
}

func (Sender) TableName() string { return "senders" }

type SyncRun struct {
	ID                         string  `gorm:"column:id;primaryKey" json:"id"`
	Status                     string  `gorm:"column:status;not null" json:"status"`
	Phase                      string  `gorm:"column:phase;not null" json:"phase"`
	LlmProvider                string  `gorm:"column:llm_provider;not null" json:"llm_provider"`
	LlmModel                   string  `gorm:"column:llm_model;not null" json:"llm_model"`
	FetchedCount               int     `gorm:"column:fetched_count;not null;default:0" json:"fetched_count"`
	CleanedCount               int     `gorm:"column:cleaned_count;not null;default:0" json:"cleaned_count"`
	ImportedCount              int     `gorm:"column:imported_count;not null;default:0" json:"imported_count"`
	MergedCount                int     `gorm:"column:merged_count;not null;default:0" json:"merged_count"`
	SkippedCount               int     `gorm:"column:skipped_count;not null;default:0" json:"skipped_count"`
	FailedCount                int     `gorm:"column:failed_count;not null;default:0" json:"failed_count"`
	HasMoreEmails              int     `gorm:"column:has_more_emails;not null;default:0" json:"has_more_emails"`
	ProvisionalAccountsCreated int     `gorm:"column:provisional_accounts_created;not null;default:0" json:"provisional_accounts_created"`
	ErrorMessage               *string `gorm:"column:error_message" json:"error_message"`
	StartedAtUtc               string  `gorm:"column:started_at_utc;not null" json:"started_at_utc"`
	FinishedAtUtc              *string `gorm:"column:finished_at_utc" json:"finished_at_utc"`
}

func (SyncRun) TableName() string { return "sync_runs" }

type RawEmail struct {
	ID             string  `gorm:"column:id;primaryKey" json:"id"`
	GmailMessageID string  `gorm:"column:gmail_message_id;not null;unique" json:"gmail_message_id"`
	SenderID       *string `gorm:"column:sender_id" json:"sender_id"`
	FromAddress    string  `gorm:"column:from_address;not null" json:"from_address"`
	Subject        string  `gorm:"column:subject;not null" json:"subject"`
	CleanedText    string  `gorm:"column:cleaned_text;not null" json:"cleaned_text"`
	InternalDateMs int64   `gorm:"column:internal_date_ms;not null" json:"internal_date_ms"`
	ReceivedAtUtc  string  `gorm:"column:received_at_utc;not null" json:"received_at_utc"`
	Status         string  `gorm:"column:status;not null" json:"status"`
	ErrorMessage   *string `gorm:"column:error_message" json:"error_message"`
}

func (RawEmail) TableName() string { return "raw_emails" }

type EmailExtraction struct {
	ID              string  `gorm:"column:id;primaryKey" json:"id"`
	EmailID         string  `gorm:"column:email_id;not null;unique" json:"email_id"`
	EmailKind       string  `gorm:"column:email_kind;not null" json:"email_kind"`
	AmountMinor     *int64  `gorm:"column:amount_minor" json:"amount_minor"`
	Currency        *string `gorm:"column:currency" json:"currency"`
	OccurredAtUtc   *string `gorm:"column:occurred_at_utc" json:"occurred_at_utc"`
	MerchantRaw     *string `gorm:"column:merchant_raw" json:"merchant_raw"`
	MerchantNorm    *string `gorm:"column:merchant_norm" json:"merchant_norm"`
	Category        *string `gorm:"column:category" json:"category"`
	CardLast4       *string `gorm:"column:card_last4" json:"card_last4"`
	PaymentMethodRaw *string `gorm:"column:payment_method_raw" json:"payment_method_raw"`
	BankAccountHint *string `gorm:"column:bank_account_hint" json:"bank_account_hint"`
	LlmAccountID    *string `gorm:"column:llm_account_id" json:"llm_account_id"`
	ReferenceCode   *string `gorm:"column:reference_code" json:"reference_code"`
	ExtractionModel string  `gorm:"column:extraction_model;not null" json:"extraction_model"`
	RawLlmJSON      string  `gorm:"column:raw_llm_json;not null" json:"raw_llm_json"`
}

func (EmailExtraction) TableName() string { return "email_extractions" }

type Transaction struct {
	ID             string  `gorm:"column:id;primaryKey" json:"id"`
	SourceType     string  `gorm:"column:source_type;not null" json:"source_type"`
	AccountID      *string `gorm:"column:account_id" json:"account_id"`
	AmountMinor    int64   `gorm:"column:amount_minor;not null" json:"amount_minor"`
	Currency       string  `gorm:"column:currency;not null" json:"currency"`
	OccurredAtUtc  string  `gorm:"column:occurred_at_utc;not null" json:"occurred_at_utc"`
	Merchant       string  `gorm:"column:merchant;not null" json:"merchant"`
	Category       string  `gorm:"column:category;not null" json:"category"`
	ManualOverride int     `gorm:"column:manual_override;not null;default:0" json:"manual_override"`
	Reviewed       int     `gorm:"column:reviewed;not null" json:"reviewed"`
	Note           *string `gorm:"column:note" json:"note"`
	CreatedAtUtc   string  `gorm:"column:created_at_utc;not null" json:"created_at_utc"`
	UpdatedAtUtc   string  `gorm:"column:updated_at_utc;not null" json:"updated_at_utc"`
}

func (Transaction) TableName() string { return "transactions" }

type TransactionSource struct {
	ID            string  `gorm:"column:id;primaryKey" json:"id"`
	TransactionID string  `gorm:"column:transaction_id;not null" json:"transaction_id"`
	EmailID       string  `gorm:"column:email_id;not null;unique" json:"email_id"`
	Role          string  `gorm:"column:role;not null" json:"role"`
	MatchScore    float64 `gorm:"column:match_score;not null;default:1.0" json:"match_score"`
	MatchState    string  `gorm:"column:match_state;not null" json:"match_state"`
}

func (TransactionSource) TableName() string { return "transaction_sources" }
