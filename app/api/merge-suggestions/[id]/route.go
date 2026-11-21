package main

import (
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	switch body.Action {
	case "accept":
		survivorID, err := domain.ConfirmMergeSuggestion(gdb, id)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"survivorId": survivorID})
	case "reject":
		if err := domain.DismissMergeSuggestion(gdb, id); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		httpx.WriteError(w, http.StatusBadRequest, "Unsupported action (expected accept or reject).")
	}
}
