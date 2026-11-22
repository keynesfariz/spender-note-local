CREATE TABLE IF NOT EXISTS app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  country_code TEXT,
  timezone TEXT,
  currency TEXT,
  llm_provider TEXT NOT NULL DEFAULT 'openai_compatible' CHECK (llm_provider IN ('openai_compatible')),
  llm_base_url TEXT NOT NULL DEFAULT 'http://localhost:9931/v1',
  llm_model TEXT,
  llm_timeout_ms INTEGER NOT NULL DEFAULT 60000 CHECK (llm_timeout_ms > 0),
  sync_batch_limit INTEGER NOT NULL DEFAULT 50 CHECK (sync_batch_limit >= 1 AND sync_batch_limit <= 500)
) STRICT;

CREATE TABLE IF NOT EXISTS gmail_oauth (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  status TEXT NOT NULL DEFAULT 'disconnected' CHECK (status IN ('disconnected', 'connected', 'expired')),
  email_address TEXT,
  access_token TEXT,
  refresh_token TEXT,
  expires_at_utc TEXT,
  scope TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS oauth_states (
  state TEXT PRIMARY KEY,
  code_verifier TEXT NOT NULL,
  expires_at_utc TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('credit_card', 'debit_card', 'digital_wallet')),
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  initial_balance_minor INTEGER,
  initial_balance_as_of_utc TEXT NOT NULL CHECK (length(initial_balance_as_of_utc) = 20 AND initial_balance_as_of_utc LIKE '%Z'),
  last4 TEXT CHECK (last4 IS NULL OR (length(last4) = 4 AND last4 GLOB '[0-9][0-9][0-9][0-9]')),
  statement_day INTEGER CHECK (statement_day IS NULL OR (statement_day >= 1 AND statement_day <= 31)),
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('provisional', 'confirmed')),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE IF NOT EXISTS account_identifiers (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('card_last4', 'bank_account', 'alias')),
  value TEXT NOT NULL CHECK (length(trim(value)) > 0),
  normalized_value TEXT NOT NULL CHECK (length(normalized_value) > 0),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  UNIQUE (account_id, kind, normalized_value)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_account_identifiers_kind_norm
  ON account_identifiers(kind, normalized_value);

CREATE INDEX IF NOT EXISTS idx_account_identifiers_account
  ON account_identifiers(account_id);

CREATE TABLE IF NOT EXISTS account_reconciliations (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  computed_balance_minor INTEGER NOT NULL,
  actual_balance_minor INTEGER NOT NULL,
  adjustment_minor INTEGER NOT NULL,
  note TEXT,
  recorded_at_utc TEXT NOT NULL CHECK (length(recorded_at_utc) = 20 AND recorded_at_utc LIKE '%Z')
) STRICT;

CREATE INDEX IF NOT EXISTS idx_account_reconciliations_acct_time
  ON account_reconciliations(account_id, recorded_at_utc DESC);

CREATE TABLE IF NOT EXISTS senders (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  role_hint TEXT NOT NULL DEFAULT 'auto' CHECK (role_hint IN ('auto', 'card_charge', 'receipt')),
  default_account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  backfill_from_utc TEXT NOT NULL CHECK (length(backfill_from_utc) = 20 AND backfill_from_utc LIKE '%Z'),
  last_internal_date_ms INTEGER,
  last_sync_at_utc TEXT
) STRICT;

CREATE TABLE IF NOT EXISTS sync_runs (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'partial_error', 'failed', 'cancelled', 'interrupted')),
  phase TEXT NOT NULL CHECK (phase IN ('preflight', 'fetching', 'syncing', 'extracting', 'done')),
  llm_provider TEXT NOT NULL,
  llm_model TEXT NOT NULL,
  fetched_count INTEGER NOT NULL DEFAULT 0,
  cleaned_count INTEGER NOT NULL DEFAULT 0,
  imported_count INTEGER NOT NULL DEFAULT 0,
  merged_count INTEGER NOT NULL DEFAULT 0,
  skipped_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  has_more_emails INTEGER NOT NULL DEFAULT 0 CHECK (has_more_emails IN (0, 1)),
  provisional_accounts_created INTEGER NOT NULL DEFAULT 0,
  error_message TEXT,
  started_at_utc TEXT NOT NULL CHECK (length(started_at_utc) = 20 AND started_at_utc LIKE '%Z'),
  finished_at_utc TEXT
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_runs_single_running
  ON sync_runs(status) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS raw_emails (
  id TEXT PRIMARY KEY,
  gmail_message_id TEXT NOT NULL UNIQUE,
  sender_id TEXT REFERENCES senders(id) ON DELETE SET NULL,
  from_address TEXT NOT NULL,
  subject TEXT NOT NULL,
  cleaned_text TEXT NOT NULL,
  internal_date_ms INTEGER NOT NULL,
  received_at_utc TEXT NOT NULL CHECK (length(received_at_utc) = 20 AND received_at_utc LIKE '%Z'),
  status TEXT NOT NULL CHECK (status IN ('pending', 'extracted', 'non_transaction', 'failed', 'dismissed')),
  error_message TEXT
) STRICT;

CREATE INDEX IF NOT EXISTS idx_raw_emails_status_date
  ON raw_emails(status, internal_date_ms ASC);

CREATE TABLE IF NOT EXISTS email_extractions (
  id TEXT PRIMARY KEY,
  email_id TEXT NOT NULL UNIQUE REFERENCES raw_emails(id) ON DELETE CASCADE,
  email_kind TEXT NOT NULL CHECK (email_kind IN ('card_charge', 'receipt', 'non_transaction')),
  amount_minor INTEGER,
  currency TEXT,
  occurred_at_utc TEXT,
  merchant_raw TEXT,
  merchant_norm TEXT,
  category TEXT,
  card_last4 TEXT,
  payment_method_raw TEXT,
  bank_account_hint TEXT,
  llm_account_id TEXT,
  reference_code TEXT,
  extraction_model TEXT NOT NULL,
  raw_llm_json TEXT NOT NULL,
  CHECK (
    email_kind = 'non_transaction'
    OR (amount_minor > 0 AND currency IS NOT NULL AND occurred_at_utc IS NOT NULL)
  )
) STRICT;

CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY,
  source_type TEXT NOT NULL CHECK (source_type IN ('gmail', 'manual')),
  account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  occurred_at_utc TEXT NOT NULL CHECK (length(occurred_at_utc) = 20 AND occurred_at_utc LIKE '%Z'),
  merchant TEXT NOT NULL,
  category TEXT NOT NULL,
  manual_override INTEGER NOT NULL DEFAULT 0 CHECK (manual_override IN (0, 1)),
  reviewed INTEGER NOT NULL DEFAULT 1 CHECK (reviewed IN (0, 1)),
  note TEXT,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  updated_at_utc TEXT NOT NULL CHECK (length(updated_at_utc) = 20 AND updated_at_utc LIKE '%Z')
) STRICT;

CREATE INDEX IF NOT EXISTS idx_transactions_occurred
  ON transactions(occurred_at_utc DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_account_occurred
  ON transactions(account_id, occurred_at_utc);
CREATE INDEX IF NOT EXISTS idx_transactions_occurred_reviewed
  ON transactions(occurred_at_utc DESC, reviewed);

CREATE TABLE IF NOT EXISTS transaction_sources (
  id TEXT PRIMARY KEY,
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  email_id TEXT NOT NULL UNIQUE REFERENCES raw_emails(id) ON DELETE RESTRICT,
  role TEXT NOT NULL CHECK (role IN ('card_charge', 'receipt')),
  match_score REAL NOT NULL DEFAULT 1.0,
  match_state TEXT NOT NULL CHECK (match_state IN ('auto', 'confirmed', 'single'))
) STRICT;

CREATE INDEX IF NOT EXISTS idx_transaction_sources_tx
  ON transaction_sources(transaction_id);
