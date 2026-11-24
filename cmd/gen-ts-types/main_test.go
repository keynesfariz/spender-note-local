package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"portspendernote/internal/llm"
)

func TestWriteStructFields_RespectsJsonTagsAndOmitsIgnoredFields(t *testing.T) {
	type sample struct {
		Visible  string  `json:"visible"`
		Secret   string  `json:"-"`
		Optional *string `json:"opt,omitempty"`
	}

	var buf bytes.Buffer
	writeStructFields(&buf, reflect.TypeOf(sample{}), nil, map[reflect.Type]string{})
	got := buf.String()

	if !strings.Contains(got, "  visible: string\n") {
		t.Errorf("expected visible field in output, got:\n%s", got)
	}
	if !strings.Contains(got, "  opt?: string | null\n") {
		t.Errorf("expected optional nullable field in output, got:\n%s", got)
	}
	if strings.Contains(got, "Secret") || strings.Contains(got, "secret") {
		t.Errorf("expected json:\"-\" field to be omitted, got:\n%s", got)
	}
}

func TestDefaultCategoriesMatchFrontendDefinitions(t *testing.T) {
	tsFile := filepath.Join("..", "..", "lib", "categories.ts")
	raw, err := os.ReadFile(tsFile)
	if err != nil {
		t.Fatalf("read %s: %v", tsFile, err)
	}
	src := string(raw)
	start := strings.Index(src, "DEFAULT_CATEGORY_DEFINITIONS")
	if start == -1 {
		t.Fatalf("could not find DEFAULT_CATEGORY_DEFINITIONS in %s", tsFile)
	}
	end := strings.Index(src[start:], "]")
	if end == -1 {
		t.Fatalf("could not find closing bracket of DEFAULT_CATEGORY_DEFINITIONS in %s", tsFile)
	}

	re := regexp.MustCompile(`\{\s*name:\s*"([^"]+)"`)
	matches := re.FindAllStringSubmatch(src[start:start+end], -1)
	var frontendNames []string
	for _, m := range matches {
		frontendNames = append(frontendNames, m[1])
	}

	if !slices.Equal(llm.DefaultCategories, frontendNames) {
		t.Fatalf("llm.DefaultCategories (%v) does not match %s (%v)", llm.DefaultCategories, tsFile, frontendNames)
	}
}

func TestGenerateTypeScript_MatchesCommittedFile(t *testing.T) {
	repoFile := filepath.Join("..", "..", "lib", "api", "types.generated.ts")
	existing, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("read %s: %v", repoFile, err)
	}
	if string(existing) != generateTypeScript() {
		t.Fatalf("%s is out of date; run `bun run generate:types`", repoFile)
	}
}

func TestGenerateTypeScript_IncludesTransactionEnrichmentPreview(t *testing.T) {
	generated := generateTypeScript()
	for _, want := range []string{
		"export interface TransactionEnrichmentPreview {",
		"merchant: string",
		"category: string",
		"note: string | null",
		"occurred_at_utc: string | null",
		"existing_tags: TransactionTagItem[]",
		"draft_tag_names: string[]",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("expected generated TypeScript to contain %q", want)
		}
	}
}
