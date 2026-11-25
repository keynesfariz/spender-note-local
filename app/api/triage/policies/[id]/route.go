package main

import (
	"errors"
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
	"portspendernote/internal/httpx"
)

func PATCH(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id := r.PathValue("id")
	var input domain.UpdateTriagePolicyInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := domain.UpdateTriagePolicy(gdb, id, input)
	if err != nil {
		writePolicyRouteError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, updated)
}

func DELETE(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id := r.PathValue("id")
	res, err := domain.DeleteTriagePolicy(gdb, id)
	if err != nil {
		writePolicyRouteError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, res)
}

func writePolicyRouteError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	var nfe *domain.NotFoundError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusBadRequest, ve.Error())
	case errors.As(err, &nfe):
		httpx.WriteError(w, http.StatusNotFound, nfe.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
