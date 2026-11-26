package main

import (
	"errors"
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

func GET(w http.ResponseWriter, r *http.Request) {
	if !httpx.AllowLocalRequest(w, r, false) {
		return
	}
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	body, err := domain.GetSkippedPaymentEmailBody(gdb.WithContext(r.Context()), r.PathValue("emailId"))
	if err != nil {
		var notFound *domain.NotFoundError
		if errors.As(err, &notFound) {
			httpx.WriteError(w, http.StatusNotFound, notFound.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"cleanedText": body})
}
