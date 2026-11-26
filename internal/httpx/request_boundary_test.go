package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalRequestBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, host, origin, media string
		write                     bool
		status                    int
	}{
		{name: "local preview", host: "localhost:3000", status: 200},
		{name: "untrusted preview host", host: "example.test:3000", status: 403},
		{name: "local transfer", host: "127.0.0.1:3000", origin: "http://127.0.0.1:3000", media: "application/json; charset=utf-8", write: true, status: 200},
		{name: "untrusted origin", host: "127.0.0.1:3000", origin: "https://example.test", media: "application/json", write: true, status: 403},
		{name: "non-json action", host: "127.0.0.1:3000", media: "text/plain", write: true, status: 415},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.media)
			w := httptest.NewRecorder()
			ok := AllowLocalRequest(w, r, tc.write)
			if w.Code != tc.status || ok != (tc.status == 200) {
				t.Fatalf("decision = %v, status = %d; want %d", ok, w.Code, tc.status)
			}
		})
	}
}
