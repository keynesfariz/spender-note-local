package main

import (
	"encoding/json"
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

func PATCH(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := r.PathValue("id")
	var rawMap map[string]json.RawMessage
	if err := httpx.DecodeJSON(w, r, &rawMap); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var patch domain.UpdateSenderInput
	if b, ok := rawMap["email"]; ok {
		var s string
		if json.Unmarshal(b, &s) == nil {
			patch.Email = &s
		}
	}
	if b, ok := rawMap["role_hint"]; ok {
		var s string
		if json.Unmarshal(b, &s) == nil {
			patch.RoleHint = &s
		}
	}
	if b, ok := rawMap["default_account_id"]; ok {
		patch.HasDefaultAccountID = true
		var s *string
		if json.Unmarshal(b, &s) == nil {
			patch.DefaultAccountID = s
		}
	}
	if b, ok := rawMap["enabled"]; ok {
		var v bool
		if json.Unmarshal(b, &v) == nil {
			patch.Enabled = &v
		}
	}
	if b, ok := rawMap["backfill_from_utc"]; ok {
		var s string
		if json.Unmarshal(b, &s) == nil {
			patch.BackfillFromUtc = &s
		}
	}

	sender, err := domain.UpdateSender(gdb, id, patch)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sender)
}

func DELETE(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := r.PathValue("id")
	if err := domain.DeleteSender(gdb, id); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
