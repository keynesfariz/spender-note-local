# Country Configuration Files

Every supported country in PortSpenderNote is defined by a single JSON file in this directory (for example, `ID.json` for Indonesia).

These files are auto-discovered on startup and act as the sole source of truth for:

- The Settings country and timezone pickers
- Local LLM extraction prompt rules
- Local billing-statement email filter phrases

## Adding Your Country

1. Copy `ID.json` to `<CODE>.json` using your two-letter uppercase ISO 3166-1 alpha-2 country code (for example, `JP.json`, `DE.json`, `SG.json`).
2. Update the fields using `country.schema.json` for editor autocomplete and validation.
3. Restart the app (`bun run dev` or `bun run start`). No Go or TypeScript source files need to be edited.

Startup validation is strict: unknown fields, invalid IANA timezones, invalid 3-letter ISO 4217 currency codes, duplicate country codes, or a filename that does not match `"code"` will fail fast with an error naming the file and field.

## Fields

| Field | Required | Description |
|---|---|---|
| `$schema` | No | `"./country.schema.json"` for editor validation. |
| `code` | Yes | Two-letter uppercase ISO country code matching the filename (`<CODE>.json`). |
| `name` | Yes | Display name shown in Settings. |
| `currency` | Yes | Three-letter uppercase ISO 4217 default currency code. |
| `defaultTimezone` | Yes | Default IANA timezone; must appear in `timezones`. |
| `timezones` | Yes | Non-empty list of valid IANA timezones selectable in Settings. |
| `llmPromptRules` | Yes | List of prompt rule bullets appended after universal rules (may be `[]`). |
| `billingStatementFilter` | Yes | Object with `subjectPhrases`, `bodyPrimaryCues`, and `bodySecondaryCues` arrays (each may be `[]`). |

## What Universal Defaults Already Cover

You do **not** need to repeat these in your country file — they already run for every country:

- **Universal LLM prompt rules:** ISO `YYYY-MM-DD` transaction date, `HH:mm` / `HH:mm:ss` time, explicit email timezone offset parsing, fallback to the user's configured timezone, unformatted `amount_decimal`, and 3-letter ISO 4217 currency codes.
- **Universal English billing-statement filter phrases:**
  - Subjects: `billing statement`, `e-statement`, `estatement`, `credit card statement`, `monthly statement`, `account statement`
  - Body primary cues: `billing statement`, `e-statement`, `total billing`, `statement balance`, `new balance`
  - Body secondary cues: `minimum payment`, `due date`, `payment due date`
- **Currency decimal exponents:** Standard ISO 4217 exponents (such as `JPY`, `KRW`, `IDR`, `VND`, `KWD`, `BHD`) are handled globally so foreign-currency transactions format accurately for every country.
