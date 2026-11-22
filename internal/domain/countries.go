package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

type BillingStatementFilter struct {
	SubjectPhrases    []string `json:"subjectPhrases"`
	BodyPrimaryCues   []string `json:"bodyPrimaryCues"`
	BodySecondaryCues []string `json:"bodySecondaryCues"`
}

type CountryPack struct {
	Schema                 string                 `json:"$schema,omitempty"`
	Code                   string                 `json:"code"`
	Name                   string                 `json:"name"`
	Currency               string                 `json:"currency"`
	DefaultTimezone        string                 `json:"defaultTimezone"`
	Timezones              []string               `json:"timezones"`
	LlmPromptRules         []string               `json:"llmPromptRules"`
	BillingStatementFilter BillingStatementFilter `json:"billingStatementFilter"`
}

func (p *CountryPack) LocaleInfo() CountryLocaleInfo {
	tzs := make([]string, len(p.Timezones))
	copy(tzs, p.Timezones)
	return CountryLocaleInfo{
		Code:            p.Code,
		Name:            p.Name,
		Currency:        p.Currency,
		DefaultTimezone: p.DefaultTimezone,
		Timezones:       tzs,
	}
}

type CountryRegistry struct {
	ordered []CountryPack
	byCode  map[string]int
}

func (r *CountryRegistry) Catalog() []CountryLocaleInfo {
	out := make([]CountryLocaleInfo, len(r.ordered))
	for i := range r.ordered {
		out[i] = r.ordered[i].LocaleInfo()
	}
	return out
}

func (r *CountryRegistry) FindLocaleByCode(code string) *CountryLocaleInfo {
	pack := r.FindPackByCode(code)
	if pack == nil {
		return nil
	}
	info := pack.LocaleInfo()
	return &info
}

func (r *CountryRegistry) FindPackByCode(code string) *CountryPack {
	upper := strings.ToUpper(strings.TrimSpace(code))
	idx, ok := r.byCode[upper]
	if !ok {
		return nil
	}
	pack := r.ordered[idx]
	return &pack
}

func GetCountriesDir() string {
	if envDir := strings.TrimSpace(os.Getenv("COUNTRIES_DIR")); envDir != "" {
		return envDir
	}
	if dir, ok := findCountriesDirFromCwd(); ok {
		return dir
	}
	return "countries"
}

func findCountriesDirFromCwd() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, "countries")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func LoadCountryPacksFromDir(dir string) (*CountryRegistry, error) {
	realRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("countries directory %q: %w", dir, err)
	}
	rootInfo, err := os.Stat(realRoot)
	if err != nil || !rootInfo.IsDir() {
		return nil, fmt.Errorf("countries directory %q is not a readable directory", dir)
	}

	entries, err := os.ReadDir(realRoot)
	if err != nil {
		return nil, fmt.Errorf("countries directory %q: %w", dir, err)
	}

	type candidateFile struct {
		fileName string
		fullPath string
	}
	var jsonFiles []candidateFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".schema.json") {
			continue
		}
		candidatePath := filepath.Join(realRoot, name)
		realTarget, err := filepath.EvalSymlinks(candidatePath)
		if err != nil {
			return nil, fmt.Errorf("%s: failed to resolve path: %w", candidatePath, err)
		}
		if !isStrictlyInside(realRoot, realTarget) {
			return nil, fmt.Errorf("%s: symlink target escapes countries directory", candidatePath)
		}
		st, err := os.Stat(realTarget)
		if err != nil {
			return nil, fmt.Errorf("%s: stat error: %w", candidatePath, err)
		}
		if st.IsDir() {
			continue
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: country configuration must be a regular file", candidatePath)
		}
		jsonFiles = append(jsonFiles, candidateFile{fileName: name, fullPath: realTarget})
	}
	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].fileName < jsonFiles[j].fileName
	})

	if len(jsonFiles) == 0 {
		return nil, fmt.Errorf("countries directory %q contains no country configuration files", dir)
	}

	packs := make([]CountryPack, 0, len(jsonFiles))
	byCode := make(map[string]int, len(jsonFiles))

	for _, f := range jsonFiles {
		pack, err := parseAndValidateCountryFile(f.fullPath, f.fileName)
		if err != nil {
			return nil, err
		}
		if prevIdx, exists := byCode[pack.Code]; exists {
			return nil, fmt.Errorf("%s: duplicate country code %q (already defined for %q)", f.fullPath, pack.Code, packs[prevIdx].Name)
		}
		byCode[pack.Code] = len(packs)
		packs = append(packs, pack)
	}

	sort.Slice(packs, func(i, j int) bool {
		return packs[i].Code < packs[j].Code
	})
	for i := range packs {
		byCode[packs[i].Code] = i
	}

	return &CountryRegistry{
		ordered: packs,
		byCode:  byCode,
	}, nil
}

func parseAndValidateCountryFile(fullPath, fileName string) (CountryPack, error) {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return CountryPack{}, fmt.Errorf("%s: read error: %w", fullPath, err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var pack CountryPack
	if err := dec.Decode(&pack); err != nil {
		return CountryPack{}, fmt.Errorf("%s: invalid JSON: %w", fullPath, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return CountryPack{}, fmt.Errorf("%s: unexpected trailing content after JSON object", fullPath)
	}

	if err := validateCountryPack(fullPath, fileName, &pack); err != nil {
		return CountryPack{}, err
	}
	return pack, nil
}

func validateCountryPack(fullPath, fileName string, pack *CountryPack) error {
	pack.Code = strings.TrimSpace(pack.Code)
	if !countryCodeRe.MatchString(pack.Code) {
		return fmt.Errorf("%s: field \"code\" must be a 2-letter uppercase ISO country code, got %q", fullPath, pack.Code)
	}
	stem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if stem != pack.Code {
		return fmt.Errorf("%s: filename stem %q must match field \"code\" %q", fullPath, stem, pack.Code)
	}

	pack.Name = strings.TrimSpace(pack.Name)
	if pack.Name == "" {
		return fmt.Errorf("%s: field \"name\" is required and cannot be empty", fullPath)
	}

	currency, err := NormalizeCurrencyCode(pack.Currency)
	if err != nil {
		return fmt.Errorf("%s: field \"currency\": %w", fullPath, err)
	}
	pack.Currency = currency

	if len(pack.Timezones) == 0 {
		return fmt.Errorf("%s: field \"timezones\" must contain at least one valid IANA timezone", fullPath)
	}
	seenTz := make(map[string]struct{}, len(pack.Timezones))
	normalizedTzs := make([]string, 0, len(pack.Timezones))
	for i, rawTz := range pack.Timezones {
		tz := strings.TrimSpace(rawTz)
		if tz == "" {
			return fmt.Errorf("%s: field \"timezones[%d]\" cannot be empty", fullPath, i)
		}
		if strings.EqualFold(tz, "Local") {
			return fmt.Errorf("%s: field \"timezones[%d]\": \"Local\" is not a valid IANA timezone", fullPath, i)
		}
		if _, err := LoadTimezone(tz); err != nil {
			return fmt.Errorf("%s: field \"timezones[%d]\": %w", fullPath, i, err)
		}
		if _, dup := seenTz[tz]; dup {
			return fmt.Errorf("%s: field \"timezones\" contains duplicate timezone %q", fullPath, tz)
		}
		seenTz[tz] = struct{}{}
		normalizedTzs = append(normalizedTzs, tz)
	}
	pack.Timezones = normalizedTzs

	pack.DefaultTimezone = strings.TrimSpace(pack.DefaultTimezone)
	if pack.DefaultTimezone == "" {
		return fmt.Errorf("%s: field \"defaultTimezone\" is required and cannot be empty", fullPath)
	}
	if strings.EqualFold(pack.DefaultTimezone, "Local") {
		return fmt.Errorf("%s: field \"defaultTimezone\": \"Local\" is not a valid IANA timezone", fullPath)
	}
	if _, err := LoadTimezone(pack.DefaultTimezone); err != nil {
		return fmt.Errorf("%s: field \"defaultTimezone\": %w", fullPath, err)
	}
	if _, ok := seenTz[pack.DefaultTimezone]; !ok {
		return fmt.Errorf("%s: field \"defaultTimezone\" (%q) must be listed in \"timezones\"", fullPath, pack.DefaultTimezone)
	}

	if pack.LlmPromptRules == nil {
		return fmt.Errorf("%s: field \"llmPromptRules\" is required (use [] if none)", fullPath)
	}
	cleanedRules, err := validateNonEmptyStringSlice(fullPath, "llmPromptRules", pack.LlmPromptRules, false)
	if err != nil {
		return err
	}
	pack.LlmPromptRules = cleanedRules

	if pack.BillingStatementFilter.SubjectPhrases == nil {
		return fmt.Errorf("%s: field \"billingStatementFilter.subjectPhrases\" is required (use [] if none)", fullPath)
	}
	if pack.BillingStatementFilter.BodyPrimaryCues == nil {
		return fmt.Errorf("%s: field \"billingStatementFilter.bodyPrimaryCues\" is required (use [] if none)", fullPath)
	}
	if pack.BillingStatementFilter.BodySecondaryCues == nil {
		return fmt.Errorf("%s: field \"billingStatementFilter.bodySecondaryCues\" is required (use [] if none)", fullPath)
	}

	if pack.BillingStatementFilter.SubjectPhrases, err = validateNonEmptyStringSlice(fullPath, "billingStatementFilter.subjectPhrases", pack.BillingStatementFilter.SubjectPhrases, true); err != nil {
		return err
	}
	if pack.BillingStatementFilter.BodyPrimaryCues, err = validateNonEmptyStringSlice(fullPath, "billingStatementFilter.bodyPrimaryCues", pack.BillingStatementFilter.BodyPrimaryCues, true); err != nil {
		return err
	}
	if pack.BillingStatementFilter.BodySecondaryCues, err = validateNonEmptyStringSlice(fullPath, "billingStatementFilter.bodySecondaryCues", pack.BillingStatementFilter.BodySecondaryCues, true); err != nil {
		return err
	}

	return nil
}

func validateNonEmptyStringSlice(fullPath, fieldName string, items []string, lower bool) ([]string, error) {
	out := make([]string, 0, len(items))
	for i, raw := range items {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%s: field \"%s[%d]\" cannot be empty", fullPath, fieldName, i)
		}
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		out = append(out, trimmed)
	}
	return out, nil
}

var (
	defaultCountryRegistryMu sync.RWMutex
	defaultCountryRegistry   *CountryRegistry
)

func init() {
	reg, err := LoadCountryPacksFromDir(GetCountriesDir())
	if err != nil {
		panic(fmt.Sprintf("failed to load country configuration: %v", err))
	}
	defaultCountryRegistry = reg
}

func ListCountryCatalog() []CountryLocaleInfo {
	defaultCountryRegistryMu.RLock()
	reg := defaultCountryRegistry
	defaultCountryRegistryMu.RUnlock()
	return reg.Catalog()
}

func FindCountryByCode(code string) *CountryLocaleInfo {
	defaultCountryRegistryMu.RLock()
	reg := defaultCountryRegistry
	defaultCountryRegistryMu.RUnlock()
	return reg.FindLocaleByCode(code)
}

func FindCountryPackByCode(code string) *CountryPack {
	defaultCountryRegistryMu.RLock()
	reg := defaultCountryRegistry
	defaultCountryRegistryMu.RUnlock()
	return reg.FindPackByCode(code)
}
