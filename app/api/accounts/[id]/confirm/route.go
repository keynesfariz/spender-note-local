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
	acct, err := domain.ConfirmAccount(gdb, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, acct)
}
