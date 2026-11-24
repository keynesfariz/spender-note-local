DROP INDEX IF EXISTS idx_email_extractions_triage_signature_state;
DROP INDEX IF EXISTS idx_email_extractions_triage_state_occurred;

ALTER TABLE email_extractions DROP COLUMN triage_state;
ALTER TABLE email_extractions DROP COLUMN triage_body_cue;
ALTER TABLE email_extractions DROP COLUMN triage_subject_pattern;
ALTER TABLE email_extractions DROP COLUMN triage_signature_key;
ALTER TABLE email_extractions DROP COLUMN account_type_guess;

DROP TABLE IF EXISTS sender_triage_policies;
