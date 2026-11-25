package main

import (
	"errors"
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

	var input domain.TriageUndoInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	res, err := domain.UndoTriageVerdict(gdb, input)
	if err != nil {
		var ve *domain.ValidationError
		var nfe *domain.NotFoundError
		var ce *domain.ConflictError
		switch {
		case errors.As(err, &ve):
			httpx.WriteError(w, http.StatusBadRequest, ve.Error())
		case errors.As(err, &nfe):
			httpx.WriteError(w, http.StatusNotFound, nfe.Error())
		case errors.As(err, &ce):
			httpx.WriteError(w, http.StatusConflict, ce.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, res)
}
