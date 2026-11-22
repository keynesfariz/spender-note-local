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
	rawBytes, _ := json.Marshal(rawMap)
	var patch domain.UpdateAccountInput
	if err := json.Unmarshal(rawBytes, &patch); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid account update payload.")
		return
	}
	if _, ok := rawMap["initial_balance_minor"]; ok {
		patch.HasInitialBalance = true
	}
	if _, ok := rawMap["last4"]; ok {
		patch.HasLast4 = true
	}
	if _, ok := rawMap["statement_day"]; ok {
		patch.HasStatementDay = true
	}
	if _, ok := rawMap["identifiers"]; ok {
		patch.HasIdentifiers = true
	}

	acct, err := domain.UpdateAccount(gdb, id, patch)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, acct)
}

func DELETE(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := r.PathValue("id")
	if err := domain.DeleteAccount(gdb, id); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
