package main

import (
	"net/http"

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
	q := r.URL.Query()
	items, err := domain.ListTransactions(gdb, domain.ListTransactionsParams{
		Month:        q.Get("month"),
		Category:     q.Get("category"),
		AccountID:    q.Get("accountId"),
		Search:       q.Get("search"),
		UnmergedOnly: q.Get("unmergedOnly") == "true" || q.Get("unmergedOnly") == "1",
	})
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var input domain.CreateManualTransactionInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := domain.CreateManualTransaction(gdb, input)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, tx)
}
