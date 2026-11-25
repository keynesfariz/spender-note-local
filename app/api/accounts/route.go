package main

import (
	"encoding/json"
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
	if r.URL.Query().Get("view") == "options" {
		options, err := domain.ListAccountOptions(gdb.WithContext(r.Context()))
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, options)
		return
	}
	accounts, err := domain.ListAccounts(gdb)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, accounts)
}

func POST(w http.ResponseWriter, r *http.Request) {
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
	var input domain.CreateAccountInput
	if err := json.Unmarshal(rawBytes, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid account payload.")
		return
	}
	if _, ok := rawMap["identifiers"]; ok {
		input.HasIdentifiers = true
	}
	acct, err := domain.CreateAccount(gdb, input)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, acct)
}
