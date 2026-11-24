package gmail

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"portspendernote/internal/db"
)

type oauthTestTransport func(*http.Request) (*http.Response, error)

func (f oauthTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRefreshKeepsNewerConnectionState(t *testing.T) {
	cases := []struct {
		name              string
		statusCode        int
		body              string
		disconnect        bool
		wantStatus        string
		wantAccess        string
		wantRefresh       string
		wantConnectionErr bool
	}{
		{
			name: "successful refresh", statusCode: http.StatusOK, wantStatus: "connected",
			body:       `{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}`,
			wantAccess: "refreshed-access", wantRefresh: "refreshed-refresh",
		},
		{
			name: "disconnect during refresh", statusCode: http.StatusOK, disconnect: true,
			body:       `{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}`,
			wantStatus: "disconnected", wantConnectionErr: true,
		},
		{
			name: "reconnect before invalid grant", statusCode: http.StatusBadRequest,
			body: `{"error":"invalid_grant"}`, wantStatus: "connected",
			wantAccess: "new-access", wantRefresh: "new-refresh", wantConnectionErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gdb, err := db.OpenDatabase(":memory:", false)
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := gdb.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := gdb.Exec(`
				UPDATE gmail_oauth SET status = 'connected', email_address = 'owner@example.com',
				    access_token = 'old-access', refresh_token = 'old-refresh' WHERE id = 1`).Error; err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
			t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
			originalTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			http.DefaultTransport = oauthTestTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == googleRevokeURL {
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
				}
				if req.URL.String() != googleTokenURL {
					t.Fatalf("unexpected OAuth request: %s", req.URL)
				}
				if tc.disconnect {
					if err := DisconnectGmail(req.Context(), gdb); err != nil {
						t.Fatal(err)
					}
				} else if tc.wantConnectionErr {
					if err := gdb.Exec(`
						UPDATE gmail_oauth SET access_token = 'new-access',
						    refresh_token = 'new-refresh' WHERE id = 1`).Error; err != nil {
						t.Fatal(err)
					}
				}
				return &http.Response{StatusCode: tc.statusCode, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})

			token, err := GetValidAccessToken(context.Background(), gdb, true)
			if tc.wantConnectionErr {
				if !errors.Is(err, errGmailConnectionChanged) || token != "" {
					t.Fatalf("expected stale refresh rejection, got token %q and error %v", token, err)
				}
			} else if err != nil || token != tc.wantAccess {
				t.Fatalf("expected refreshed token %q, got %q and error %v", tc.wantAccess, token, err)
			}
			var got db.GmailOAuth
			if err := gdb.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus {
				t.Fatalf("connection state = %q, want %q", got.Status, tc.wantStatus)
			}
			if tc.disconnect {
				if got.AccessToken != nil || got.RefreshToken != nil {
					t.Fatal("stale refresh repopulated disconnected token fields")
				}
			} else if got.AccessToken == nil || *got.AccessToken != tc.wantAccess ||
				got.RefreshToken == nil || *got.RefreshToken != tc.wantRefresh {
				t.Fatal("stored OAuth tokens did not match the current connection")
			}
		})
	}
}
