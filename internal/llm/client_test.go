package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	got, err := NormalizeBaseURL("")
	if err != nil || got != DefaultBaseURL {
		t.Fatalf("expected default URL %q, got %q (err=%v)", DefaultBaseURL, got, err)
	}
	got, err = NormalizeBaseURL("http://localhost:9931/v1/")
	if err != nil || got != "http://localhost:9931/v1" {
		t.Fatalf("expected trimmed URL, got %q (err=%v)", got, err)
	}
	if _, err := NormalizeBaseURL("ftp://localhost:9931/v1"); err == nil {
		t.Fatalf("expected error for non-http scheme")
	}
}

func TestClient_ListModelsAndPreflight(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "qwen2.5-3b"},
				{"id": "llama-3.2"},
				{"id": "qwen2.5-3b"},
			},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL+"/v1", 5*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 2 || models[0] != "llama-3.2" || models[1] != "qwen2.5-3b" {
		t.Fatalf("unexpected sorted unique models: %v", models)
	}
	if err := c.Preflight(context.Background(), "qwen2.5-3b"); err != nil {
		t.Fatalf("expected preflight ok, got %v", err)
	}
	if err := c.Preflight(context.Background(), ""); !IsRuntimeError(err) {
		t.Fatalf("expected RuntimeError for empty model, got %v", err)
	}
}

func TestClient_ExtractTransaction_JsonSchemaAndCachePrompt(t *testing.T) {
	var seenCachePrompt bool
	var seenResponseFormatType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if v, ok := body["cache_prompt"].(bool); ok {
			seenCachePrompt = v
		}
		if rf, ok := body["response_format"].(map[string]any); ok {
			if typ, ok := rf["type"].(string); ok {
				seenResponseFormatType = typ
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `<think>reasoning</think>{"email_kind":"card_charge","amount_decimal":"12.50","currency":"USD","transaction_date":"2026-11-22","transaction_time":"14:30","timezone_offset":"-05:00","merchant":"Coffee Shop","category":"Dining","card_last4":"1234","payment_method":"Visa","bank_account_hint":null,"matched_account_id":null,"account_type_guess":"credit_card","reference_code":"REF-1"}`,
					},
				},
			},
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL+"/v1", 5*time.Second)
	cur := "USD"
	res, err := c.ExtractTransaction(context.Background(), "qwen2.5", ExtractInput{
		EmailID:       "e1",
		FromAddress:   "alerts@bank.com",
		Subject:       "Card charge",
		CleanedText:   "You spent $12.50 at Coffee Shop",
		ReceivedAtUtc: "2026-11-22T19:35:00Z",
		UserTimezone:  "America/New_York",
		UserCurrency:  &cur,
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !seenCachePrompt {
		t.Fatalf("expected cache_prompt=true")
	}
	if seenResponseFormatType != "json_schema" {
		t.Fatalf("expected response_format.type=json_schema, got %q", seenResponseFormatType)
	}
	if res.AmountMinor != 1250 || res.Currency != "USD" || res.OccurredAtUtc != "2026-11-22T19:30:00Z" || res.MerchantRaw != "Coffee Shop" {
		t.Fatalf("unexpected extraction result: %+v", res)
	}
}

func TestClient_ExtractTransaction_FallsBackToJsonObjectOn400(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		rf, _ := body["response_format"].(map[string]any)
		if n == 1 && rf["type"] == "json_schema" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "json_schema unsupported"}})
			return
		}
		if rf["type"] != "json_object" {
			t.Fatalf("expected fallback response_format=json_object, got %v", rf)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"email_kind":"non_transaction","amount_decimal":null,"currency":null,"transaction_date":null,"transaction_time":null,"timezone_offset":null,"merchant":null,"category":null,"card_last4":null,"payment_method":null,"bank_account_hint":null,"matched_account_id":null,"account_type_guess":null,"reference_code":null}`}},
			},
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL+"/v1", 5*time.Second)
	res, err := c.ExtractTransaction(context.Background(), "other-model", ExtractInput{
		EmailID:       "e2",
		FromAddress:   "noreply@bank.com",
		Subject:       "OTP",
		CleanedText:   "Your OTP is 123456",
		ReceivedAtUtc: "2026-11-22T10:00:00Z",
		UserTimezone:  "UTC",
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if attempts.Load() != 2 || res.EmailKind != "non_transaction" {
		t.Fatalf("expected 2 attempts and non_transaction, got attempts=%d kind=%s", attempts.Load(), res.EmailKind)
	}
}

func TestClient_ExtractTransaction_ErrorClassification(t *testing.T) {
	badJSONSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "not json"}}},
		})
	}))
	defer badJSONSrv.Close()

	c, _ := NewClient(badJSONSrv.URL+"/v1", 5*time.Second)
	_, err := c.ExtractTransaction(context.Background(), "m", ExtractInput{
		EmailID:       "e3",
		ReceivedAtUtc: "2026-11-22T10:00:00Z",
		UserTimezone:  "UTC",
	})
	if !IsExtractionError(err) {
		t.Fatalf("expected ExtractionError for invalid JSON, got %T: %v", err, err)
	}

	badJSONSrv.Close()
	_, err = c.ExtractTransaction(context.Background(), "m", ExtractInput{
		EmailID:       "e4",
		ReceivedAtUtc: "2026-11-22T10:00:00Z",
		UserTimezone:  "UTC",
	})
	if !IsRuntimeError(err) {
		t.Fatalf("expected RuntimeError for closed server, got %T: %v", err, err)
	}
}
