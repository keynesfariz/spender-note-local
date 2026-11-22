package main

import (
	"net/http"

	"portspendernote/internal/db"
	"portspendernote/internal/httpx"
	"portspendernote/internal/syncrunner"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDBWithRecovery()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := syncrunner.GetSyncStatus(gdb)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func POST(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDBWithRecovery()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch body.Action {
	case "trigger":
		runID, err := syncrunner.TriggerSync(gdb)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
	case "retry":
		runID, err := syncrunner.RetryPendingAndFailedExtractions(gdb)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
	case "cancel":
		cancelled := syncrunner.CancelSync(gdb)
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"cancelled": cancelled})
	case "dismiss":
		count, err := syncrunner.DismissPendingQueue(gdb)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int64{"dismissedCount": count})
	default:
		httpx.WriteError(w, http.StatusBadRequest, "Unsupported sync action.")
	}
}
