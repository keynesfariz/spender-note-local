package main

import (
	"net/http"
	"net/url"

	"portspendernote/internal/db"
	"portspendernote/internal/gmail"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		http.Redirect(w, r, "/settings?gmail=error&error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	q := r.URL.Query()
	_, err = gmail.HandleGmailCallback(r.Context(), gdb, r, q.Get("code"), q.Get("state"), q.Get("error"))
	if err != nil {
		http.Redirect(w, r, "/settings?gmail=error&error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/settings?gmail=connected", http.StatusFound)
}
