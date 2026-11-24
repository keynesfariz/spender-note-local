package main

import (
	"encoding/json"
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

	var rawMap map[string]json.RawMessage
	if err := httpx.DecodeJSON(w, r, &rawMap); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	rawBytes, _ := json.Marshal(rawMap)

	var envelope struct {
		Mode              string `json:"mode"`
		KeepTransactionID string `json:"keepTransactionId"`
		DropTransactionID string `json:"dropTransactionId"`
	}
	if err := json.Unmarshal(rawBytes, &envelope); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid merge payload.")
		return
	}

	switch envelope.Mode {
	case "duplicate":
		tx, err := domain.DuplicateMergeTransactions(gdb, envelope.KeepTransactionID, envelope.DropTransactionID)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, tx)
	case "combine":
		var input domain.CombineTransactionsInput
		if err := json.Unmarshal(rawBytes, &input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "Invalid combine payload.")
			return
		}
		if _, ok := rawMap["account_id"]; ok {
			input.HasAccountID = true
		}
		if _, ok := rawMap["note"]; ok {
			input.HasNote = true
		}
		if _, ok := rawMap["tag_ids"]; ok {
			input.HasTagIDs = true
		}
		tx, err := domain.CombineTransactions(gdb, input)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, tx)
	default:
		httpx.WriteError(w, http.StatusBadRequest, "Invalid merge mode (expected 'duplicate' or 'combine').")
	}
}
