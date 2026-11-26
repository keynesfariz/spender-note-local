CREATE TABLE app_settings (
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

CREATE TABLE gmail_oauth (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  status TEXT NOT NULL DEFAULT 'disconnected' CHECK (status IN ('disconnected', 'connected', 'expired')),
  email_address TEXT,
  access_token TEXT,
  refresh_token TEXT,
  expires_at_utc TEXT,
  scope TEXT
) STRICT;

CREATE TABLE oauth_states (
  state TEXT PRIMARY KEY,
  code_verifier TEXT NOT NULL,
  expires_at_utc TEXT NOT NULL
) STRICT;

CREATE TABLE accounts (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('credit_card', 'debit_card', 'digital_wallet')),
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  initial_balance_minor INTEGER,
  initial_balance_as_of_utc TEXT NOT NULL CHECK (length(initial_balance_as_of_utc) = 20 AND initial_balance_as_of_utc LIKE '%Z'),
  statement_day INTEGER CHECK (statement_day IS NULL OR (statement_day >= 1 AND statement_day <= 31)),
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('provisional', 'confirmed')),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE account_identifiers (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  value TEXT NOT NULL CHECK (length(trim(value)) > 0),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  UNIQUE (account_id, value)
) STRICT;

CREATE TABLE account_reconciliations (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  computed_balance_minor INTEGER NOT NULL,
  actual_balance_minor INTEGER NOT NULL,
  adjustment_minor INTEGER NOT NULL,
  note TEXT,
  recorded_at_utc TEXT NOT NULL CHECK (length(recorded_at_utc) = 20 AND recorded_at_utc LIKE '%Z')
) STRICT;

CREATE INDEX idx_account_reconciliations_acct_time
  ON account_reconciliations(account_id, recorded_at_utc DESC);

CREATE TABLE senders (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  role_hint TEXT NOT NULL DEFAULT 'auto' CHECK (role_hint IN ('auto', 'card_charge', 'receipt')),
  default_account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  backfill_from_utc TEXT NOT NULL CHECK (length(backfill_from_utc) = 20 AND backfill_from_utc LIKE '%Z'),
  last_internal_date_ms INTEGER,
  last_sync_at_utc TEXT
) STRICT;

-- CSV imports can run on connections with foreign key enforcement disabled.
CREATE TRIGGER senders_validate_default_account_insert
BEFORE INSERT ON senders
WHEN NEW.default_account_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.default_account_id)
BEGIN
  SELECT RAISE(ABORT, 'sender default account does not exist');
END;

CREATE TRIGGER senders_validate_default_account_update
BEFORE UPDATE OF default_account_id ON senders
WHEN NEW.default_account_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.default_account_id)
BEGIN
  SELECT RAISE(ABORT, 'sender default account does not exist');
END;

CREATE TABLE sync_runs (
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

CREATE UNIQUE INDEX idx_sync_runs_single_running
  ON sync_runs(status) WHERE status = 'running';

CREATE TABLE raw_emails (
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

CREATE INDEX idx_raw_emails_status_date
  ON raw_emails(status, internal_date_ms ASC);

CREATE TABLE email_extractions (
  id TEXT PRIMARY KEY,
  email_id TEXT NOT NULL UNIQUE REFERENCES raw_emails(id) ON DELETE CASCADE,
  email_kind TEXT NOT NULL CHECK (email_kind IN ('card_charge', 'receipt', 'non_transaction')),
  amount_minor INTEGER,
  currency TEXT,
  occurred_at_utc TEXT,
  merchant_raw TEXT,
  category TEXT,
  payment_method_raw TEXT,
  bank_account_hint TEXT,
  llm_account_id TEXT,
  reference_code TEXT,
  extraction_model TEXT NOT NULL,
  raw_llm_json TEXT NOT NULL,
  account_type_guess TEXT,
  triage_signature_key TEXT,
  triage_subject_pattern TEXT,
  triage_body_cue TEXT,
  triage_state TEXT CHECK (triage_state IS NULL OR triage_state IN ('ghost', 'ignored', 'promoted')),
  CHECK (
    email_kind = 'non_transaction'
    OR (amount_minor > 0 AND currency IS NOT NULL AND occurred_at_utc IS NOT NULL)
  )
) STRICT;

CREATE TABLE categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0),
  normalized_name TEXT NOT NULL UNIQUE CHECK (length(normalized_name) > 0),
  icon TEXT NOT NULL CHECK (length(trim(icon)) > 0),
  is_system INTEGER NOT NULL DEFAULT 0 CHECK (is_system IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE transactions (
  id TEXT PRIMARY KEY,
  source_type TEXT NOT NULL CHECK (source_type IN ('gmail', 'manual')),
  account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
  currency TEXT NOT NULL CHECK (length(currency) = 3),
  occurred_at_utc TEXT NOT NULL CHECK (length(occurred_at_utc) = 20 AND occurred_at_utc LIKE '%Z'),
  merchant TEXT NOT NULL,
  category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
  remark TEXT,
  remark_user_edited INTEGER NOT NULL DEFAULT 0 CHECK (remark_user_edited IN (0, 1)),
  manual_override INTEGER NOT NULL DEFAULT 0 CHECK (manual_override IN (0, 1)),
  reviewed INTEGER NOT NULL DEFAULT 1 CHECK (reviewed IN (0, 1)),
  note TEXT,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  updated_at_utc TEXT NOT NULL CHECK (length(updated_at_utc) = 20 AND updated_at_utc LIKE '%Z'),
  kind TEXT NOT NULL DEFAULT 'expense'
  CHECK (kind IN ('expense', 'income', 'transfer')),
  destination_account_id TEXT REFERENCES accounts(id) ON DELETE RESTRICT,
  destination_amount_minor INTEGER,
  destination_currency TEXT,
  source_posted_at_utc TEXT
  CHECK (source_posted_at_utc IS NULL OR (kind = 'transfer' AND length(source_posted_at_utc) = 20 AND source_posted_at_utc LIKE '%Z')),
  destination_posted_at_utc TEXT CHECK (
  (destination_posted_at_utc IS NULL OR (kind = 'transfer' AND length(destination_posted_at_utc) = 20 AND destination_posted_at_utc LIKE '%Z'))
  AND (
    (kind = 'transfer' AND account_id IS NOT NULL AND destination_account_id IS NOT NULL
      AND account_id <> destination_account_id
      AND destination_amount_minor IS NOT NULL AND destination_amount_minor > 0
      AND destination_currency IS NOT NULL AND length(destination_currency) = 3
      AND (currency <> destination_currency OR amount_minor = destination_amount_minor))
    OR (kind IN ('expense', 'income') AND destination_account_id IS NULL
      AND destination_amount_minor IS NULL AND destination_currency IS NULL
      AND (kind <> 'income' OR account_id IS NOT NULL))
  )
),
  ambiguous_source_amount INTEGER NOT NULL DEFAULT 0
  CHECK (ambiguous_source_amount IN (0, 1))
) STRICT;

CREATE INDEX idx_transactions_occurred
  ON transactions(occurred_at_utc DESC);
CREATE INDEX idx_transactions_account_occurred
  ON transactions(account_id, occurred_at_utc);
CREATE INDEX idx_transactions_occurred_reviewed
  ON transactions(occurred_at_utc DESC, reviewed);

CREATE INDEX idx_transactions_destination_account ON transactions(destination_account_id)
  WHERE destination_account_id IS NOT NULL;

CREATE TABLE transfer_requests (
  id TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  transaction_id TEXT REFERENCES transactions(id) ON DELETE SET NULL
) STRICT;

CREATE TABLE transaction_sources (
  id TEXT PRIMARY KEY,
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  email_id TEXT NOT NULL UNIQUE REFERENCES raw_emails(id) ON DELETE RESTRICT,
  role TEXT NOT NULL CHECK (role IN ('card_charge', 'receipt')),
  match_score REAL NOT NULL DEFAULT 1.0,
  match_state TEXT NOT NULL CHECK (match_state IN ('auto', 'confirmed', 'single'))
) STRICT;

CREATE INDEX idx_transaction_sources_tx
  ON transaction_sources(transaction_id);

CREATE INDEX idx_transactions_currency_occurred
  ON transactions(currency, occurred_at_utc);

CREATE INDEX idx_raw_emails_sender_date
  ON raw_emails(sender_id, internal_date_ms);

CREATE INDEX idx_transactions_category_occurred
  ON transactions(category_id, occurred_at_utc);

CREATE TABLE tags (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0),
  normalized_name TEXT NOT NULL UNIQUE CHECK (length(normalized_name) > 0),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE transaction_tags (
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  PRIMARY KEY (transaction_id, tag_id)
) STRICT;

CREATE INDEX idx_transaction_tags_tag
  ON transaction_tags(tag_id, transaction_id);

CREATE TABLE sender_triage_policies (
    id TEXT PRIMARY KEY,
    sender_email TEXT NOT NULL,
    signature_key TEXT NOT NULL,
    subject_pattern TEXT NOT NULL,
    body_cue TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL CHECK (action IN ('admit', 'ignore')),
    created_at_utc TEXT NOT NULL,
    updated_at_utc TEXT NOT NULL,
    UNIQUE (sender_email, signature_key)
) STRICT;

CREATE INDEX idx_email_extractions_triage_state_occurred
    ON email_extractions(triage_state, occurred_at_utc DESC);

CREATE INDEX idx_email_extractions_triage_signature_state
    ON email_extractions(triage_signature_key, triage_state);

INSERT INTO categories (id, name, normalized_name, icon, is_system, sort_order, created_at_utc) VALUES
  ('cat-housing', 'Housing', 'housing', 'Home', 0, 1, '2026-01-01T00:00:00Z'),
  ('cat-bills-utilities', 'Bills & Utilities', 'bills & utilities', 'Zap', 0, 2, '2026-01-01T00:00:00Z'),
  ('cat-food', 'Food', 'food', 'Croissant', 0, 3, '2026-01-01T00:00:00Z'),
  ('cat-dining-out', 'Dining Out', 'dining out', 'HandPlatter', 0, 4, '2026-01-01T00:00:00Z'),
  ('cat-entertainment', 'Entertainment', 'entertainment', 'Film', 0, 5, '2026-01-01T00:00:00Z'),
  ('cat-shopping', 'Shopping', 'shopping', 'ShoppingCart', 0, 6, '2026-01-01T00:00:00Z'),
  ('cat-groceries', 'Groceries', 'groceries', 'Carton', 0, 7, '2026-01-01T00:00:00Z'),
  ('cat-personal-care', 'Personal Care', 'personal care', 'PerfumeBottle', 0, 8, '2026-01-01T00:00:00Z'),
  ('cat-health', 'Health', 'health', 'Cross', 0, 9, '2026-01-01T00:00:00Z'),
  ('cat-education', 'Education', 'education', 'GraduationCap', 0, 10, '2026-01-01T00:00:00Z'),
  ('cat-transport', 'Transport', 'transport', 'Car', 0, 11, '2026-01-01T00:00:00Z'),
  ('cat-travel', 'Travel', 'travel', 'Plane', 0, 12, '2026-01-01T00:00:00Z'),
  ('cat-other', 'Other', 'other', 'Ellipsis', 1, 999, '2026-01-01T00:00:00Z');
