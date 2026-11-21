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
	rows, err := domain.ListAccountReconciliations(gdb, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rows)
}

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var input domain.ReconcileAccountInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	input.AccountID = r.PathValue("id")
	res, err := domain.ReconcileAccount(gdb, input)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, res)
}
