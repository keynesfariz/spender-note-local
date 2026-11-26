ALTER TABLE transactions ADD COLUMN kind TEXT NOT NULL DEFAULT 'expense'
  CHECK (kind IN ('expense', 'income', 'transfer'));
ALTER TABLE transactions ADD COLUMN destination_account_id TEXT REFERENCES accounts(id) ON DELETE RESTRICT;
ALTER TABLE transactions ADD COLUMN destination_amount_minor INTEGER;
ALTER TABLE transactions ADD COLUMN destination_currency TEXT;
ALTER TABLE transactions ADD COLUMN source_posted_at_utc TEXT
  CHECK (source_posted_at_utc IS NULL OR (kind = 'transfer' AND length(source_posted_at_utc) = 20 AND source_posted_at_utc LIKE '%Z'));
ALTER TABLE transactions ADD COLUMN destination_posted_at_utc TEXT CHECK (
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
);
ALTER TABLE transactions ADD COLUMN ambiguous_source_amount INTEGER NOT NULL DEFAULT 0
  CHECK (ambiguous_source_amount IN (0, 1));
-- An old override does not record whether its amount came from a Combine.
UPDATE transactions SET ambiguous_source_amount = 1 WHERE manual_override = 1;
CREATE INDEX idx_transactions_destination_account ON transactions(destination_account_id)
  WHERE destination_account_id IS NOT NULL;

CREATE TABLE transfer_requests (
  id TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  transaction_id TEXT REFERENCES transactions(id) ON DELETE SET NULL
) STRICT;
