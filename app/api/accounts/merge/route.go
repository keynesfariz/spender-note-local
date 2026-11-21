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
		SourceAccountID string `json:"sourceAccountId"`
		TargetAccountID string `json:"targetAccountId"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := domain.MergeAccounts(gdb, body.SourceAccountID, body.TargetAccountID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
