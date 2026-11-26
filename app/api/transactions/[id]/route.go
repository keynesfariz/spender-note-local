package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id := r.PathValue("id")
	query := r.URL.Query()
	if query.Has("sourceEmailId") {
		body, err := domain.GetTransactionSourceBody(gdb.WithContext(r.Context()), id, query.Get("sourceEmailId"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "Source email not found.")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, body)
		return
	}
	detail, err := domain.GetTransactionInspector(gdb.WithContext(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

func PATCH(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var rawMap map[string]json.RawMessage
	if err := httpx.DecodeJSON(w, r, &rawMap); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rawBytes, _ := json.Marshal(rawMap)
	var patch domain.UpdateTransactionInput
	if err := json.Unmarshal(rawBytes, &patch); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid transaction update payload.")
		return
	}
	if _, ok := rawMap["account_id"]; ok {
		patch.HasAccountID = true
	}
	if _, ok := rawMap["note"]; ok {
		patch.HasNote = true
	}
	if _, ok := rawMap["tag_ids"]; ok {
		patch.HasTagIDs = true
	}

	tx, err := domain.UpdateTransaction(gdb, r.PathValue("id"), patch)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tx)
}

func DELETE(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := domain.DeleteTransaction(gdb, r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
