import "server-only"

export interface CountryPromptContext {
  userCountry?: string | null
  countryPromptRules?: readonly string[]
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

export function getCountryPromptRules(ctx: CountryPromptContext): string[] {
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

  if (ctx.countryPromptRules) {
    for (const rule of ctx.countryPromptRules) {
      const trimmed = rule.trim()
      if (trimmed) {
        rules.push(trimmed)
      }
    }
  }

  return rules
}
