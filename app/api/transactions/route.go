package main

import (
	"net/http"
	"strconv"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

var triageBackfill domain.TriageBackfill

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	var showSkippedPtr *bool
	if rawShowSkipped := q.Get("showSkipped"); rawShowSkipped == "false" || rawShowSkipped == "0" {
		v := false
		showSkippedPtr = &v
	}
	res, err := domain.ListTransactions(gdb.WithContext(r.Context()), domain.ListTransactionsParams{
		Backfill:       &triageBackfill,
		LedgerRange:    q.Get("ledgerRange"),
		Month:          q.Get("month"),
		StartDate:      q.Get("startDate"),
		EndDate:        q.Get("endDate"),
		Category:       q.Get("category"),
		Tag:            q.Get("tag"),
		AccountID:      q.Get("accountId"),
		Search:         q.Get("search"),
		UnreviewedOnly: q.Get("unreviewedOnly") == "true" || q.Get("unreviewedOnly") == "1",
		ShowSkipped:    showSkippedPtr,
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
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
