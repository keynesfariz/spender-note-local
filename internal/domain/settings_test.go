package domain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"portspendernote/internal/db"
)

func TestGetAppSettings_LiveModelsAndOfflineFallback(t *testing.T) {
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	var probes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		probes.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "qwen2.5-3b"}, {"id": "gemma-3"}},
		})
	}))
	defer srv.Close()

	baseURL := srv.URL + "/v1/"
	model := "qwen2.5-3b"
	timeout := 30000
	res, err := UpdateLlmSettings(gdb, UpdateLlmInput{
		LlmBaseURL:   &baseURL,
		LlmModel:     &model,
		LlmTimeoutMs: &timeout,
	})
	if err != nil {
		t.Fatalf("update llm settings: %v", err)
	}
	if res["llm_base_url"] != srv.URL+"/v1" {
		t.Fatalf("expected trailing slash trimmed, got %v", res["llm_base_url"])
	}

	config, err := GetSavedSettings(gdb)
	if err != nil || config.LlmBaseURL != srv.URL+"/v1" {
		t.Fatalf("saved settings = %+v, %v", config, err)
	}
	locale, err := GetAppLocale(gdb)
	if err != nil || locale.Timezone != nil || locale.Currency != nil {
		t.Fatalf("initial locale = %+v, %v", locale, err)
	}
	if probes.Load() != 0 {
		t.Fatal("saved settings and locale must not probe the model server")
	}

	view, err := GetAppSettings(context.Background(), gdb, "")
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !view.ServerReachable || len(view.AvailableModels) != 2 {
		t.Fatalf("expected reachable server with 2 models, got reachable=%v models=%v err=%v", view.ServerReachable, view.AvailableModels, view.ServerError)
	}

	// Offline probe must not fail GetAppSettings.
	srv.Close()
	offlineView, err := GetAppSettings(context.Background(), gdb, "")
	if err != nil {
		t.Fatalf("expected offline settings read to succeed, got %v", err)
	}
	if offlineView.ServerReachable || len(offlineView.AvailableModels) != 0 || offlineView.ServerError == nil {
		t.Fatalf("expected unreachable state with error message, got %+v", offlineView)
	}
}

func TestUpdateLlmSettings_Validation(t *testing.T) {
	gdb, err := db.OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	badURL := "ftp://localhost:9931/v1"
	if _, err := UpdateLlmSettings(gdb, UpdateLlmInput{LlmBaseURL: &badURL}); err == nil {
		t.Fatalf("expected invalid scheme to be rejected")
	}

	badTimeout := 500
	if _, err := UpdateLlmSettings(gdb, UpdateLlmInput{LlmTimeoutMs: &badTimeout}); err == nil {
		t.Fatalf("expected timeout < 1000ms to be rejected")
	}
}
