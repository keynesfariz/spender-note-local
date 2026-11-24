ALTER TABLE transactions ADD COLUMN remark TEXT;
ALTER TABLE transactions ADD COLUMN remark_user_edited INTEGER NOT NULL DEFAULT 0;

-- Legacy edits are only identifiable when both parts of a compound title
-- appear in the linked evidence and no extraction already used that title.
WITH candidates AS (
    SELECT id, merchant, trim(substr(merchant, 1, instr(merchant, ': ') - 1)) AS service,
        trim(substr(merchant, instr(merchant, ': ') + 2)) AS vendor
    FROM transactions
    WHERE manual_override = 1 AND instr(merchant, ': ') > 1
)
UPDATE transactions
SET remark = merchant, remark_user_edited = 1
WHERE id IN (
    SELECT c.id FROM candidates c
    WHERE c.service <> '' AND c.vendor <> ''
      AND EXISTS (
        SELECT 1 FROM transaction_sources ts JOIN raw_emails re ON re.id = ts.email_id
        WHERE ts.transaction_id = c.id
          AND instr(lower(coalesce(re.subject, '') || ' ' || coalesce(re.cleaned_text, '')), lower(c.service)) > 0
      )
      AND EXISTS (
        SELECT 1 FROM transaction_sources ts JOIN raw_emails re ON re.id = ts.email_id
        WHERE ts.transaction_id = c.id
          AND instr(lower(coalesce(re.subject, '') || ' ' || coalesce(re.cleaned_text, '')), lower(c.vendor)) > 0
      )
      AND NOT EXISTS (
        SELECT 1 FROM transaction_sources ts JOIN email_extractions ee ON ee.email_id = ts.email_id
        WHERE ts.transaction_id = c.id
          AND lower(trim(ee.merchant_raw)) = lower(trim(c.merchant))
      )
);
