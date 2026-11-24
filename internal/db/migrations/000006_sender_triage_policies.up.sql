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

ALTER TABLE email_extractions ADD COLUMN account_type_guess TEXT;
ALTER TABLE email_extractions ADD COLUMN triage_signature_key TEXT;
ALTER TABLE email_extractions ADD COLUMN triage_subject_pattern TEXT;
ALTER TABLE email_extractions ADD COLUMN triage_body_cue TEXT;
ALTER TABLE email_extractions ADD COLUMN triage_state TEXT CHECK (triage_state IS NULL OR triage_state IN ('ghost', 'ignored', 'promoted'));

CREATE INDEX idx_email_extractions_triage_state_occurred
    ON email_extractions(triage_state, occurred_at_utc DESC);

CREATE INDEX idx_email_extractions_triage_signature_state
    ON email_extractions(triage_signature_key, triage_state);
