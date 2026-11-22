package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
	"portspendernote/internal/llm"
	"portspendernote/internal/syncrunner"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	probeBaseURL := strings.TrimSpace(r.URL.Query().Get("llm_base_url"))
	settings, err := domain.GetAppSettings(r.Context(), gdb, probeBaseURL)
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
		LlmBaseURL     *string `json:"llm_base_url"`
		LlmModel       *string `json:"llm_model"`
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
		res, err := domain.UpdateLlmSettings(gdb, domain.UpdateLlmInput{
			LlmBaseURL:   body.LlmBaseURL,
			LlmModel:     body.LlmModel,
			LlmTimeoutMs: body.LlmTimeoutMs,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
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
		httpx.WriteError(w, http.StatusConflict, "Cannot test LLM connection while a background sync is running.")
		return
	}

	var row db.AppSetting
	if err := gdb.First(&row, 1).Error; err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Application settings are missing.")
		return
	}

	var body struct {
		LlmBaseURL   *string `json:"llm_base_url"`
		LlmModel     *string `json:"llm_model"`
		LlmTimeoutMs *int    `json:"llm_timeout_ms"`
	}
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "Invalid JSON request body.")
				return
			}
		}
	}

	baseURL := row.LlmBaseURL
	if body.LlmBaseURL != nil {
		baseURL = *body.LlmBaseURL
	}
	model := ""
	if row.LlmModel != nil {
		model = *row.LlmModel
	}
	if body.LlmModel != nil {
		model = *body.LlmModel
	}
	model = strings.TrimSpace(model)
	if model == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Select or enter a model ID before testing the connection.")
		return
	}

	timeoutMs := row.LlmTimeoutMs
	if body.LlmTimeoutMs != nil && *body.LlmTimeoutMs >= 1000 && *body.LlmTimeoutMs <= 600000 {
		timeoutMs = *body.LlmTimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	client, err := llm.NewClient(baseURL, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := client.CheckHealth(r.Context(), model); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"baseUrl": client.BaseURL,
		"model":   model,
	})
}
