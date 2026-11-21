package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return errors.New("Invalid JSON request body.")
	}
	return nil
}

func InternalBaseURL(r *http.Request) string {
	if configured := strings.TrimSpace(os.Getenv("INTERNAL_APP_URL")); configured != "" {
		return strings.TrimRight(configured, "/")
	}
	host := ""
	if r != nil {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "3000"
		}
		host = "127.0.0.1:" + port
	}
	return "http://" + host
}
