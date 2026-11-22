CREATE INDEX IF NOT EXISTS idx_transactions_currency_occurred
  ON transactions(currency, occurred_at_utc);

CREATE INDEX IF NOT EXISTS idx_raw_emails_sender_date
  ON raw_emails(sender_id, internal_date_ms);
