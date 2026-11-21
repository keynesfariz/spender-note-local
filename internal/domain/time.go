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
