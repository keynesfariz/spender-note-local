package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
	"portspendernote/internal/syncrunner"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := domain.GetAppSettings(gdb)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settings)
}

func PATCH(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var body struct {
		Section        string  `json:"section"`
		CountryCode    string  `json:"country_code"`
		Timezone       *string `json:"timezone"`
		Currency       *string `json:"currency"`
		LlmModelPath   *string `json:"llm_model_path"`
		LlmContextSize *int    `json:"llm_context_size"`
		LlmTimeoutMs   *int    `json:"llm_timeout_ms"`
		SyncBatchLimit int     `json:"sync_batch_limit"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch body.Section {
	case "locale":
		res, err := domain.UpdateLocaleSettings(gdb, domain.UpdateLocaleInput{
			CountryCode: body.CountryCode,
			Timezone:    body.Timezone,
			Currency:    body.Currency,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
	case "llm":
		res, resolvedSafePath, err := domain.UpdateLlmSettings(gdb, domain.UpdateLlmInput{
			LlmModelPath:   body.LlmModelPath,
			LlmContextSize: body.LlmContextSize,
			LlmTimeoutMs:   body.LlmTimeoutMs,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Best-effort notify node-llama-cpp warm cache to evict if mismatched
		evictPayload, _ := json.Marshal(map[string]any{
			"modelPath":   resolvedSafePath,
			"contextSize": res["llm_context_size"],
		})
		client := &http.Client{Timeout: 3 * time.Second}
		if resp, err := client.Post(httpx.InternalBaseURL(r)+"/api/llm/evict", "application/json", bytes.NewReader(evictPayload)); err == nil {
			_ = resp.Body.Close()
		}
		httpx.WriteJSON(w, http.StatusOK, res)
	case "sync":
		res, err := domain.UpdateSyncSettings(gdb, body.SyncBatchLimit)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
	default:
		httpx.WriteError(w, http.StatusBadRequest, "Unsupported settings section.")
	}
}

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := syncrunner.GetSyncStatus(gdb)
	if err == nil && status.IsRunning {
		httpx.WriteError(w, http.StatusConflict, "Cannot test model load while a background sync is running.")
		return
	}
	var row db.AppSetting
	if err := gdb.First(&row, 1).Error; err != nil || row.LlmModelPath == nil || *row.LlmModelPath == "" {
		httpx.WriteError(w, http.StatusBadRequest, "No GGUF model path configured.")
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"modelPath":   *row.LlmModelPath,
		"contextSize": row.LlmContextSize,
		"timeoutMs":   row.LlmTimeoutMs,
	})
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(httpx.InternalBaseURL(r)+"/api/llm/check", "application/json", bytes.NewReader(payload))
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "Failed to reach LLM check endpoint: "+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(raw)
}
