package main

import (
	"net/http"
	"strconv"

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

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	list := domain.ListSkippedTransactions
	if q.Get("withoutDraft") == "true" {
		list = domain.ListSkippedPaymentEmails
	}
	res, err := list(gdb.WithContext(r.Context()), page, pageSize)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, res)
}
