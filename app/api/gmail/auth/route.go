package main

import (
	"net/http"
	"net/url"
	"strings"

	"portspendernote/internal/db"
	"portspendernote/internal/gmail"
)

func GET(w http.ResponseWriter, r *http.Request) {
	gdb, err := db.GetDB()
	if err != nil {
		http.Redirect(w, r, "/settings?gmail=error&error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	loginHint := strings.TrimSpace(r.URL.Query().Get("login_hint"))
	if loginHint == "" {
		var existing db.GmailOAuth
		if err := gdb.Select("status, email_address").First(&existing, 1).Error; err == nil {
			if existing.Status != "disconnected" && existing.EmailAddress != nil && *existing.EmailAddress != "" {
				loginHint = *existing.EmailAddress
			}
		}
	}

	authURL, err := gmail.StartGmailOAuth(gdb, r, loginHint)
	if err != nil {
		http.Redirect(w, r, "/settings?gmail=error&error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}
