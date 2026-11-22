import "server-only"

export interface CountryPromptContext {
  userCountry?: string | null
  userTimezone: string
  referenceUtc?: string
}

const offsetFormatterCache = new Map<string, Intl.DateTimeFormat>()

function getOffsetFormatter(timeZone: string): Intl.DateTimeFormat {
  let formatter = offsetFormatterCache.get(timeZone)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat("en-US", {
      timeZone,
      timeZoneName: "longOffset",
    })
    offsetFormatterCache.set(timeZone, formatter)
  }
  return formatter
}

export function resolveTimezoneOffsetString(
  timeZone: string,
  referenceUtc?: string
): string | null {
  const refMs = referenceUtc ? Date.parse(referenceUtc) : Date.now()
  const date = Number.isNaN(refMs) ? new Date() : new Date(refMs)
  try {
    const parts = getOffsetFormatter(timeZone).formatToParts(date)
    const tzPart = parts.find((p) => p.type === "timeZoneName")?.value ?? ""
    if (tzPart === "GMT" || tzPart === "UTC") {
      return "Z"
    }
    const match = /^(?:GMT|UTC)([+-])(\d{2}):(\d{2})$/.exec(tzPart)
    if (match) {
      return `${match[1]}${match[2]}:${match[3]}`
    }
    return null
  } catch {
    return null
  }
}

const COUNTRY_RULE_BUILDERS: Record<
  string,
  (ctx: CountryPromptContext) => string[]
> = {
  ID: () => [
    '- For Indonesian Rupiah (IDR / Rp), dots or commas followed by 3 digits (e.g. "Rp36.500" or "IDR 24,500.00") are thousand separators, so return the full integer "36500" or "24500", never "36.50" or "24.50".',
    '- Indonesian timezone abbreviations map to fixed offsets: "WIB" -> "+07:00", "WITA" -> "+08:00", "WIT" -> "+09:00".',
    '- Common local payment methods and wallets include "GoPay", "OVO Cash", "ShopeePay", "DANA", and "QRIS".',
  ],
}

export function getCountryPromptRules(ctx: CountryPromptContext): string[] {
  const countryCode = ctx.userCountry?.trim().toUpperCase() || ""
  const resolvedOffset = resolveTimezoneOffsetString(
    ctx.userTimezone,
    ctx.referenceUtc
  )
  const offsetHint = resolvedOffset
    ? `${ctx.userTimezone} (${resolvedOffset})`
    : ctx.userTimezone

  const rules: string[] = [
    '- Always return transaction_date as "YYYY-MM-DD" WITH transaction_time ("HH:mm:ss" or "HH:mm") AND timezone_offset ("+HH:mm", "-HH:mm", or "Z") whenever a transaction timestamp is present in the email.',
    `- Derive timezone_offset from any explicit timezone or abbreviation shown in the email first (e.g. "UTC" -> "Z", "PST" -> "-08:00"). If the email omits a timezone, use the user's configured timezone ${offsetHint} combined with the email's timestamp.`,
  ]

  const countryBuilder = COUNTRY_RULE_BUILDERS[countryCode]
  if (countryBuilder) {
    rules.push(...countryBuilder(ctx))
  }

  return rules
}
