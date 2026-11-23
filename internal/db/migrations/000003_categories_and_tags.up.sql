CREATE TABLE IF NOT EXISTS categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0),
  normalized_name TEXT NOT NULL UNIQUE CHECK (length(normalized_name) > 0),
  icon TEXT NOT NULL CHECK (length(trim(icon)) > 0),
  is_system INTEGER NOT NULL DEFAULT 0 CHECK (is_system IN (0, 1)),
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE IF NOT EXISTS tags (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0),
  normalized_name TEXT NOT NULL UNIQUE CHECK (length(normalized_name) > 0),
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z')
) STRICT;

CREATE TABLE IF NOT EXISTS transaction_tags (
  transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  created_at_utc TEXT NOT NULL CHECK (length(created_at_utc) = 20 AND created_at_utc LIKE '%Z'),
  PRIMARY KEY (transaction_id, tag_id)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_transaction_tags_tag
  ON transaction_tags(tag_id, transaction_id);

CREATE INDEX IF NOT EXISTS idx_transactions_category_occurred
  ON transactions(category, occurred_at_utc);

INSERT OR IGNORE INTO categories (id, name, normalized_name, icon, is_system, sort_order, created_at_utc) VALUES
  ('cat-housing', 'Housing', 'housing', 'Home', 0, 1, '2026-01-01T00:00:00Z'),
  ('cat-food', 'Food', 'food', 'ShoppingCart', 0, 2, '2026-01-01T00:00:00Z'),
  ('cat-utilities', 'Utilities', 'utilities', 'Zap', 0, 3, '2026-01-01T00:00:00Z'),
  ('cat-transportation', 'Transportation', 'transportation', 'Car', 0, 4, '2026-01-01T00:00:00Z'),
  ('cat-healthcare', 'Healthcare', 'healthcare', 'HeartPulse', 0, 5, '2026-01-01T00:00:00Z'),
  ('cat-entertainment', 'Entertainment', 'entertainment', 'Ticket', 0, 6, '2026-01-01T00:00:00Z'),
  ('cat-dining-out', 'Dining Out', 'dining out', 'Utensils', 0, 7, '2026-01-01T00:00:00Z'),
  ('cat-shopping', 'Shopping', 'shopping', 'ShoppingBag', 0, 8, '2026-01-01T00:00:00Z'),
  ('cat-education', 'Education', 'education', 'GraduationCap', 0, 9, '2026-01-01T00:00:00Z'),
  ('cat-personal-care', 'Personal Care', 'personal care', 'Sparkles', 0, 10, '2026-01-01T00:00:00Z'),
  ('cat-other', 'Other', 'other', 'Ellipsis', 1, 999, '2026-01-01T00:00:00Z');

INSERT OR IGNORE INTO categories (id, name, normalized_name, icon, is_system, sort_order, created_at_utc)
SELECT
  'cat-legacy-' || lower(hex(randomblob(8))),
  MIN(trim(category)),
  lower(trim(category)),
  'Tag',
  0,
  100,
  '2026-01-01T00:00:00Z'
FROM (
  SELECT category FROM transactions WHERE length(trim(category)) > 0
  UNION ALL
  SELECT category FROM email_extractions WHERE category IS NOT NULL AND length(trim(category)) > 0
)
WHERE lower(trim(category)) NOT IN (SELECT normalized_name FROM categories)
GROUP BY lower(trim(category));
