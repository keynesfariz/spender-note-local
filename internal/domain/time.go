package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var utcIsoSecondsRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
var yearMonthRe = regexp.MustCompile(`^(\d{4})-(\d{2})$`)
var isoDateRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

func IsValidUtcIsoSeconds(value string) bool {
	if !utcIsoSecondsRe.MatchString(value) {
		return false
	}
	_, err := time.Parse("2006-01-02T15:04:05Z", value)
	return err == nil
}

func ToUtcIsoSeconds(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func NowUtcIsoSeconds() string {
	return ToUtcIsoSeconds(time.Now().UTC())
}

func UtcIsoToEpochMs(utcIso string) (int64, error) {
	if !IsValidUtcIsoSeconds(utcIso) {
		return 0, fmt.Errorf("expected fixed-width UTC ISO 8601 (YYYY-MM-DDTHH:mm:ssZ), got: %s", utcIso)
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", utcIso)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

func LoadTimezone(tz string) (*time.Location, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid IANA timezone: %s", tz)
	}
	return loc, nil
}

func GetCurrentMonthInTimezone(tz string) string {
	loc, err := LoadTimezone(tz)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
}

func GetMonthUtcBounds(monthYYYYMM string, tz string) (startUtc string, endUtc string, err error) {
	m := yearMonthRe.FindStringSubmatch(strings.TrimSpace(monthYYYYMM))
	if m == nil {
		return "", "", fmt.Errorf("invalid month format (expected YYYY-MM): %s", monthYYYYMM)
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	if month < 1 || month > 12 {
		return "", "", fmt.Errorf("invalid month number: %s", monthYYYYMM)
	}
	loc, err := LoadTimezone(tz)
	if err != nil {
		return "", "", err
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	nextYear, nextMonth := year, month+1
	if month == 12 {
		nextYear = year + 1
		nextMonth = 1
	}
	end := time.Date(nextYear, time.Month(nextMonth), 1, 0, 0, 0, 0, loc)
	return ToUtcIsoSeconds(start), ToUtcIsoSeconds(end), nil
}

func GetMonthStartUtc(monthYYYYMM string, tz string) (string, error) {
	start, _, err := GetMonthUtcBounds(monthYYYYMM, tz)
	return start, err
}

func GetDateRangeUtcBounds(startDateYYYYMMDD string, endDateYYYYMMDD string, tz string) (startUtc string, endUtc string, err error) {
	startStr := strings.TrimSpace(startDateYYYYMMDD)
	endStr := strings.TrimSpace(endDateYYYYMMDD)
	if endStr == "" {
		endStr = startStr
	}
	if startStr == "" {
		startStr = endStr
	}

	sm := isoDateRe.FindStringSubmatch(startStr)
	if sm == nil {
		return "", "", fmt.Errorf("invalid startDate format (expected YYYY-MM-DD): %s", startDateYYYYMMDD)
	}
	em := isoDateRe.FindStringSubmatch(endStr)
	if em == nil {
		return "", "", fmt.Errorf("invalid endDate format (expected YYYY-MM-DD): %s", endDateYYYYMMDD)
	}

	loc, err := LoadTimezone(tz)
	if err != nil {
		return "", "", err
	}

	startDay, err := time.ParseInLocation("2006-01-02", startStr, loc)
	if err != nil {
		return "", "", fmt.Errorf("invalid startDate: %s", startDateYYYYMMDD)
	}
	endDay, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return "", "", fmt.Errorf("invalid endDate: %s", endDateYYYYMMDD)
	}
	if endDay.Before(startDay) {
		return "", "", fmt.Errorf("endDate (%s) cannot be earlier than startDate (%s)", endStr, startStr)
	}

	endExclusive := endDay.AddDate(0, 0, 1)
	return ToUtcIsoSeconds(startDay), ToUtcIsoSeconds(endExclusive), nil
}

type BillingCycleWindow struct {
	StartDate       string
	EndDate         string
	StartUtc        string
	EndExclusiveUtc string
}

type CreditCardCycleBounds struct {
	CurrentCycle      BillingCycleWindow
	LastCycle         BillingCycleWindow
	LastStatementDate string
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func shiftYearMonth(year int, month time.Month, deltaMonths int) (int, time.Month) {
	total := year*12 + int(month-1) + deltaMonths
	y := total / 12
	m := total % 12
	if m < 0 {
		m += 12
		y--
	}
	return y, time.Month(m + 1)
}

func clampedStatementDate(year int, month time.Month, statementDay int, loc *time.Location) time.Time {
	day := statementDay
	maxDay := daysInMonth(year, month)
	if day > maxDay {
		day = maxDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func nextLocalMidnight(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, d.Location())
}

func buildBillingCycleWindow(startDay, endDay time.Time) BillingCycleWindow {
	return BillingCycleWindow{
		StartDate:       startDay.Format("2006-01-02"),
		EndDate:         endDay.Format("2006-01-02"),
		StartUtc:        ToUtcIsoSeconds(startDay),
		EndExclusiveUtc: ToUtcIsoSeconds(nextLocalMidnight(endDay)),
	}
}

func ComputeCreditCardCycleBounds(statementDay int, tz string, now time.Time) (CreditCardCycleBounds, error) {
	if statementDay < 1 || statementDay > 31 {
		return CreditCardCycleBounds{}, fmt.Errorf("statementDay must be between 1 and 31, got %d", statementDay)
	}
	loc, err := LoadTimezone(tz)
	if err != nil {
		return CreditCardCycleBounds{}, err
	}

	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	thisMonthStmt := clampedStatementDate(today.Year(), today.Month(), statementDay, loc)

	var curStart, curEnd, lastStart, lastEnd time.Time
	if !today.After(thisMonthStmt) {
		prevY, prevM := shiftYearMonth(today.Year(), today.Month(), -1)
		prevStmt := clampedStatementDate(prevY, prevM, statementDay, loc)
		prev2Y, prev2M := shiftYearMonth(today.Year(), today.Month(), -2)
		prev2Stmt := clampedStatementDate(prev2Y, prev2M, statementDay, loc)

		curStart = nextLocalMidnight(prevStmt)
		curEnd = thisMonthStmt
		lastStart = nextLocalMidnight(prev2Stmt)
		lastEnd = prevStmt
	} else {
		nextY, nextM := shiftYearMonth(today.Year(), today.Month(), 1)
		nextStmt := clampedStatementDate(nextY, nextM, statementDay, loc)
		prevY, prevM := shiftYearMonth(today.Year(), today.Month(), -1)
		prevStmt := clampedStatementDate(prevY, prevM, statementDay, loc)

		curStart = nextLocalMidnight(thisMonthStmt)
		curEnd = nextStmt
		lastStart = nextLocalMidnight(prevStmt)
		lastEnd = thisMonthStmt
	}

	return CreditCardCycleBounds{
		CurrentCycle:      buildBillingCycleWindow(curStart, curEnd),
		LastCycle:         buildBillingCycleWindow(lastStart, lastEnd),
		LastStatementDate: lastEnd.Format("2006-01-02"),
	}, nil
}
