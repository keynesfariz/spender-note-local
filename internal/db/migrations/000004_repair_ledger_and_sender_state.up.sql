UPDATE transactions
SET category = categories.name
FROM categories
WHERE categories.normalized_name = lower(trim(transactions.category))
  AND transactions.category <> categories.name;

UPDATE email_extractions
SET category = categories.name
FROM categories
WHERE categories.normalized_name = lower(trim(email_extractions.category))
  AND email_extractions.category <> categories.name;

-- Actual balances were entered in the account currency. Preserve the historical
-- numeric snapshots, which cannot be recalculated from an editable ledger.
UPDATE account_reconciliations
SET currency = accounts.currency
FROM accounts
WHERE account_reconciliations.account_id = accounts.id
  AND account_reconciliations.currency <> accounts.currency;

UPDATE senders
SET last_internal_date_ms = NULL, last_sync_at_utc = NULL
WHERE last_internal_date_ms IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM raw_emails
    WHERE raw_emails.sender_id = senders.id
      AND raw_emails.from_address = senders.email COLLATE NOCASE
      AND raw_emails.internal_date_ms >= senders.last_internal_date_ms
  );
