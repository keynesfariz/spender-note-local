package httpx

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// AllowLocalRequest guards local ledger and email-evidence endpoints.
func AllowLocalRequest(w http.ResponseWriter, r *http.Request, jsonWrite bool) bool {
	host, err := url.Parse("http://" + r.Host)
	if err != nil || host.User != nil || host.Host != r.Host || host.Path != "" || host.RawQuery != "" || host.Fragment != "" {
		WriteError(w, http.StatusForbidden, "Use the app's local address.")
		return false
	}
	switch strings.ToLower(host.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
	default:
		WriteError(w, http.StatusForbidden, "Use the app's local address.")
		return false
	}
	if !jsonWrite {
		return true
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if !strings.EqualFold(origin, scheme+"://"+r.Host) {
			WriteError(w, http.StatusForbidden, "This request must come from the app.")
			return false
		}
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		WriteError(w, http.StatusForbidden, "This request must come from the app.")
		return false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		WriteError(w, http.StatusUnsupportedMediaType, "Send the request as JSON.")
		return false
	}
	return true
}
