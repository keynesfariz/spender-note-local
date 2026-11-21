import {
  addDays,
  addMonths,
  differenceInCalendarDays,
  format,
  formatDistanceToNow,
  isValid,
  parse,
  parseISO,
} from 'date-fns';

export const CURRENCY_EXPONENTS: Record<string, number> = {
  JPY: 0,
  KRW: 0,
  IDR: 0,
  VND: 0,
  KWD: 3,
  BHD: 3,
  OMR: 3,
  JOD: 3,
  TND: 3,
};

const currencyFormatterCache = new Map<
  string,
  { formatter: Intl.NumberFormat; exponent: number }
>();
const dateTimeFormatterCache = new Map<string, Intl.DateTimeFormat>();

export function getCurrencyExponentClient(currency: string): number {
  const code = currency.trim().toUpperCase();
  if (Object.hasOwn(CURRENCY_EXPONENTS, code)) {
    return CURRENCY_EXPONENTS[code];
  }
  return getCurrencyFormatterEntry(code)?.exponent ?? 2;
}

function getCurrencyFormatterEntry(
  currency: string,
): { formatter: Intl.NumberFormat; exponent: number } | null {
  const code = currency.trim().toUpperCase();
  const cached = currencyFormatterCache.get(code);
  if (cached) return cached;
  try {
    const exponent = Object.hasOwn(CURRENCY_EXPONENTS, code)
      ? CURRENCY_EXPONENTS[code]
      : (new Intl.NumberFormat('en-US', {
          style: 'currency',
          currency: code,
        }).resolvedOptions().maximumFractionDigits ?? 2);
    const formatter = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: code,
      minimumFractionDigits: exponent,
      maximumFractionDigits: exponent,
    });
    const entry = { formatter, exponent };
    currencyFormatterCache.set(code, entry);
    return entry;
  } catch {
    return null;
  }
}

function getDateTimeFormatter(
  locale: string,
  timeZone: string,
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormat {
  const key = `${locale}|${timeZone}|${JSON.stringify(options)}`;
  let formatter = dateTimeFormatterCache.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, { ...options, timeZone });
    dateTimeFormatterCache.set(key, formatter);
  }
  return formatter;
}

export function formatMinorCurrency(
  amountMinor: number,
  currency: string,
): string {
  const code = currency.trim().toUpperCase();
  const entry = getCurrencyFormatterEntry(code);
  const exponent = entry?.exponent ?? getCurrencyExponentClient(code);
  const value = amountMinor / 10 ** exponent;
  if (entry) {
    return entry.formatter.format(value);
  }
  return `${code} ${value.toFixed(exponent)}`;
}

export function formatSignedMinor(
  amountMinor: number,
  currency: string,
): string {
  const formatted = formatMinorCurrency(amountMinor, currency);
  return amountMinor > 0 ? `+${formatted}` : formatted;
}

export function normalizeDecimalAmountString(
  raw: string,
  exponent: number,
): string {
  const trimmed = raw
    .trim()
    .replace(/^(?:[A-Za-z]{1,4}|\$|€|£|¥|₩|₫|₱|₹|฿)\s*/u, '')
    .replace(/\s*(?:[A-Za-z]{2,4})$/u, '')
    .trim();

  if (exponent === 0) {
    const dotGrouped = /^(-)?(\d{1,3}(?:\.\d{3})+)(?:,(0+))?$/.exec(trimmed);
    if (dotGrouped) {
      return `${dotGrouped[1] ?? ''}${dotGrouped[2].replaceAll('.', '')}`;
    }
    const commaGrouped = /^(-)?(\d[\d,]*)(?:\.(0+))?$/.exec(trimmed);
    if (commaGrouped) {
      return `${commaGrouped[1] ?? ''}${commaGrouped[2].replaceAll(',', '')}`;
    }
    const commaZeroCents = /^(-)?(\d+),(0+)$/.exec(trimmed);
    if (commaZeroCents) {
      return `${commaZeroCents[1] ?? ''}${commaZeroCents[2]}`;
    }
  }

  return trimmed.replaceAll(',', '');
}

export function parseDecimalToMinorClient(
  raw: string,
  currency: string,
): number {
  const exponent = getCurrencyExponentClient(currency);
  const cleaned = normalizeDecimalAmountString(raw, exponent);
  if (!/^-?\d+(?:\.\d+)?$/.test(cleaned)) {
    throw new Error('Enter a valid numeric amount.');
  }
  const negative = cleaned.startsWith('-');
  const unsigned = negative ? cleaned.slice(1) : cleaned;
  const [whole, frac = ''] = unsigned.split('.');
  if (frac.length > exponent) {
    throw new Error(
      `${currency.toUpperCase()} supports at most ${exponent} decimal places.`,
    );
  }
  const paddedFrac = frac.padEnd(exponent, '0');
  const minor = Number(`${whole}${paddedFrac}`);
  if (!Number.isSafeInteger(minor)) {
    throw new Error('Amount is too large.');
  }
  return negative ? -minor : minor;
}

export function minorToDecimalInput(
  amountMinor: number,
  currency: string,
): string {
  const exponent = getCurrencyExponentClient(currency);
  const divisor = 10 ** exponent;
  return (amountMinor / divisor).toFixed(exponent);
}

export function formatUtcInTimezone(
  utcIso: string,
  timeZone: string | null | undefined,
  options: Intl.DateTimeFormatOptions = {
    dateStyle: 'medium',
    timeStyle: 'short',
  },
): string {
  const parsed = parseISO(utcIso);
  if (!isValid(parsed)) return utcIso;
  try {
    return getDateTimeFormatter('en-US', timeZone || 'UTC', options).format(
      parsed,
    );
  } catch {
    return utcIso;
  }
}

export function formatRelativeUtc(utcIso: string): string {
  const parsed = parseISO(utcIso);
  if (!isValid(parsed)) return utcIso;
  return formatDistanceToNow(parsed, { addSuffix: true });
}

export function isoDateStringToDate(dateStr: string): Date | undefined {
  const trimmed = dateStr.trim();
  if (!/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return undefined;
  const parsed = parse(trimmed, 'yyyy-MM-dd', new Date());
  return isValid(parsed) ? parsed : undefined;
}

export function dateToIsoDateString(date: Date): string {
  return format(date, 'yyyy-MM-dd');
}

export function formatDateString(dateStr: string): string {
  const parsed = isoDateStringToDate(dateStr);
  if (!parsed) return dateStr;
  return format(parsed, 'MMM d, yyyy');
}

export function formatDateRangeLabel(fromStr: string, toStr: string): string {
  const fromDate = isoDateStringToDate(fromStr);
  const toDate = isoDateStringToDate(toStr || fromStr);
  if (!fromDate) return 'Pick a date';
  if (!toDate || fromStr === toStr || !toStr) {
    return format(fromDate, 'MMM d, yyyy');
  }
  return `${format(fromDate, 'MMM d, yyyy')} – ${format(toDate, 'MMM d, yyyy')}`;
}

export function getTodayDateString(timeZone?: string | null): string {
  return utcToDateInputValue(new Date().toISOString(), timeZone);
}

export function shiftDateRange(
  fromStr: string,
  toStr: string,
  direction: -1 | 1,
): { from: string; to: string } {
  const fromDate = isoDateStringToDate(fromStr);
  const toDate = isoDateStringToDate(toStr || fromStr);
  if (!fromDate || !toDate) {
    return { from: fromStr, to: toStr || fromStr };
  }
  const spanDays = Math.max(1, differenceInCalendarDays(toDate, fromDate) + 1);
  const delta = direction * spanDays;
  return {
    from: dateToIsoDateString(addDays(fromDate, delta)),
    to: dateToIsoDateString(addDays(toDate, delta)),
  };
}

export function shiftYearMonth(yearMonth: string, delta: number): string {
  const parsed = parse(yearMonth.trim(), 'yyyy-MM', new Date());
  if (!isValid(parsed)) return yearMonth;
  return format(addMonths(parsed, delta), 'yyyy-MM');
}

export function utcToDateInputValue(
  utcIso: string,
  timeZone: string | null | undefined,
): string {
  const parsed = parseISO(utcIso);
  if (!isValid(parsed)) return '';
  try {
    const parts = getDateTimeFormatter('en-CA', timeZone || 'UTC', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).formatToParts(parsed);
    const y = parts.find((p) => p.type === 'year')?.value ?? '2026';
    const m = parts.find((p) => p.type === 'month')?.value ?? '01';
    const d = parts.find((p) => p.type === 'day')?.value ?? '01';
    return `${y}-${m}-${d}`;
  } catch {
    return utcIso.slice(0, 10);
  }
}

function getTimeZoneOffsetMs(epochMs: number, timeZone: string): number {
  const parts = getDateTimeFormatter('en-US', timeZone, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(new Date(epochMs));

  const get = (type: string) =>
    Number(parts.find((p) => p.type === type)?.value ?? 0);
  const zonedAsUtc = Date.UTC(
    get('year'),
    get('month') - 1,
    get('day'),
    get('hour') % 24,
    get('minute'),
    get('second'),
  );
  return zonedAsUtc - epochMs;
}

export function utcToTimeInputValue(
  utcIso: string,
  timeZone: string | null | undefined,
): string {
  const parsed = parseISO(utcIso);
  if (!isValid(parsed)) return '';
  try {
    const parts = getDateTimeFormatter('en-US', timeZone || 'UTC', {
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(parsed);
    const h = parts.find((p) => p.type === 'hour')?.value ?? '00';
    const m = parts.find((p) => p.type === 'minute')?.value ?? '00';
    return `${h === '24' ? '00' : h}:${m}`;
  } catch {
    return utcIso.slice(11, 16);
  }
}

export function dateTimeInputToUtcIso(
  dateStr: string,
  timeStr: string,
  timeZone?: string | null,
): string {
  const trimmedDate = dateStr.trim();
  const dateMatch = /^(\d{4})-(\d{2})-(\d{2})$/.exec(trimmedDate);
  if (!dateMatch) {
    throw new Error('Date must be in YYYY-MM-DD format.');
  }
  const trimmedTime = timeStr.trim();
  const timeMatch = /^(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(trimmedTime);
  if (!timeMatch) {
    throw new Error('Time must be in HH:mm or HH:mm:ss format.');
  }

  const year = Number(dateMatch[1]);
  const month = Number(dateMatch[2]);
  const day = Number(dateMatch[3]);
  const hour = Number(timeMatch[1]);
  const minute = Number(timeMatch[2]);
  const second = timeMatch[3] ? Number(timeMatch[3]) : 0;

  if (hour > 23 || minute > 59 || second > 59) {
    throw new Error('Enter a valid time.');
  }

  const tz = timeZone?.trim() || 'UTC';
  if (tz === 'UTC') {
    const hh = String(hour).padStart(2, '0');
    const mm = String(minute).padStart(2, '0');
    const ss = String(second).padStart(2, '0');
    return `${trimmedDate}T${hh}:${mm}:${ss}Z`;
  }

  const targetUtc = Date.UTC(year, month - 1, day, hour, minute, second);
  let guess = targetUtc;
  for (let i = 0; i < 3; i++) {
    const offset = getTimeZoneOffsetMs(guess, tz);
    const next = targetUtc - offset;
    if (next === guess) break;
    guess = next;
  }

  return new Date(guess).toISOString().replace(/\.\d{3}Z$/, 'Z');
}

export function dateInputToUtcIso(
  dateStr: string,
  timeZone?: string | null,
): string {
  return dateTimeInputToUtcIso(dateStr, '00:00:00', timeZone);
}
