package main

import (
	"errors"
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
	"portspendernote/internal/llm"
)

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var input domain.EnrichTransactionsInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	preview, err := domain.EnrichTransactionsFromSources(gdb, r.Context(), input.TransactionIDs)
	if err != nil {
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			httpx.WriteError(w, http.StatusBadRequest, ve.Error())
		case llm.IsRuntimeError(err), llm.IsExtractionError(err):
			httpx.WriteError(w, http.StatusBadGateway, err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, preview)
}
