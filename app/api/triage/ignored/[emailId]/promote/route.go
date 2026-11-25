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

	emailID := r.PathValue("emailId")
	res, err := domain.PromoteIgnoredEmail(gdb, r.Context(), emailID)
	if err != nil {
		var ve *domain.ValidationError
		var nfe *domain.NotFoundError
		var ce *domain.ConflictError
		var ue *domain.UnprocessableError
		switch {
		case errors.As(err, &ve):
			httpx.WriteError(w, http.StatusBadRequest, ve.Error())
		case errors.As(err, &nfe):
			httpx.WriteError(w, http.StatusNotFound, nfe.Error())
		case errors.As(err, &ce):
			httpx.WriteError(w, http.StatusConflict, ce.Error())
		case errors.As(err, &ue):
			httpx.WriteError(w, http.StatusUnprocessableEntity, ue.Error())
		case llm.IsRuntimeError(err), llm.IsExtractionError(err):
			httpx.WriteError(w, http.StatusBadGateway, err.Error())
		default:
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, res)
}
