package gmail

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"portspendernote/internal/db"
	"portspendernote/internal/domain"
)

const (
	GmailReadonlyScope         = "https://www.googleapis.com/auth/gmail.readonly"
	googleAuthURL              = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL             = "https://oauth2.googleapis.com/token"
	googleRevokeURL            = "https://oauth2.googleapis.com/revoke"
	gmailProfileURL            = "https://gmail.googleapis.com/gmail/v1/users/me/profile"
	oauthStateTTL              = 10 * time.Minute
	accessTokenRefreshBuffer   = 60 * time.Second
)

type AuthExpiredError struct {
	Message string
}

func (e *AuthExpiredError) Error() string { return e.Message }

type googleTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func getRequiredOAuthClientConfig() (string, string, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	if clientID == "" {
		return "", "", errors.New("GOOGLE_CLIENT_ID is not configured")
	}
	if clientSecret == "" {
		return "", "", errors.New("GOOGLE_CLIENT_SECRET is not configured")
	}
	return clientID, clientSecret, nil
}

func ResolveRedirectURI(r *http.Request) string {
	if configured := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URI")); configured != "" {
		return configured
	}
	port := "3000"
	if r != nil && r.Host != "" {
		if _, p, ok := strings.Cut(r.Host, ":"); ok && p != "" {
			port = p
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%s/api/gmail/callback", port)
}

func randomBase64URL(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func computeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func hasReadonlyScope(scopeStr string) bool {
	for _, s := range strings.Fields(scopeStr) {
		if s == GmailReadonlyScope {
			return true
		}
	}
	return false
}

func StartGmailOAuth(gdb *gorm.DB, r *http.Request, loginHint string) (string, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	if clientID == "" {
		return "", errors.New("GOOGLE_CLIENT_ID is not configured")
	}

	now := time.Now().UTC()
	nowUtc := domain.ToUtcIsoSeconds(now)
	expiresAtUtc := domain.ToUtcIsoSeconds(now.Add(oauthStateTTL))

	state, err := randomBase64URL(24)
	if err != nil {
		return "", err
	}
	verifier, err := randomBase64URL(32)
	if err != nil {
		return "", err
	}
	challenge := computeCodeChallenge(verifier)
	redirectURI := ResolveRedirectURI(r)

	err = gdb.Transaction(func(tx *gorm.DB) error {
		tx.Where("expires_at_utc <= ?", nowUtc).Delete(&db.OAuthState{})
		return tx.Create(&db.OAuthState{
			State:        state,
			CodeVerifier: verifier,
			ExpiresAtUtc: expiresAtUtc,
		}).Error
	})
	if err != nil {
		return "", err
	}

	u, _ := url.Parse(googleAuthURL)
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", GmailReadonlyScope)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	if strings.TrimSpace(loginHint) != "" {
		q.Set("login_hint", strings.TrimSpace(loginHint))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func HandleGmailCallback(ctx context.Context, gdb *gorm.DB, r *http.Request, code, state, oauthErr string) (string, error) {
	if oauthErr != "" {
		return "", fmt.Errorf("Google OAuth error: %s", oauthErr)
	}
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" {
		return "", errors.New("Missing OAuth state parameter")
	}
	if code == "" {
		return "", errors.New("Missing OAuth authorization code")
	}

	now := time.Now().UTC()
	var stateRow db.OAuthState
	var found bool
	_ = gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state = ?", state).First(&stateRow).Error; err == nil {
			found = true
			tx.Where("state = ?", state).Delete(&db.OAuthState{})
		}
		return nil
	})
	if !found {
		return "", errors.New("Invalid or unknown OAuth state")
	}
	expMs, err := domain.UtcIsoToEpochMs(stateRow.ExpiresAtUtc)
	if err != nil || expMs <= now.UnixMilli() {
		return "", errors.New("OAuth state has expired; please try connecting again")
	}

	clientID, clientSecret, err := getRequiredOAuthClientConfig()
	if err != nil {
		return "", err
	}
	redirectURI := ResolveRedirectURI(r)

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"code_verifier": {stateRow.CodeVerifier},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	tokenReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRes, err := httpClient.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("Failed to exchange OAuth code: %w", err)
	}
	defer tokenRes.Body.Close()

	var tokenData googleTokenResponse
	_ = json.NewDecoder(tokenRes.Body).Decode(&tokenData)
	if tokenRes.StatusCode < 200 || tokenRes.StatusCode >= 300 || tokenData.AccessToken == "" {
		detail := tokenData.ErrorDescription
		if detail == "" {
			detail = tokenData.Error
		}
		if detail == "" {
			detail = fmt.Sprintf("HTTP %d", tokenRes.StatusCode)
		}
		return "", fmt.Errorf("Failed to exchange OAuth code: %s", detail)
	}
	if tokenData.ExpiresIn <= 0 {
		return "", errors.New("OAuth token response missing valid expires_in")
	}
	if !hasReadonlyScope(tokenData.Scope) {
		return "", fmt.Errorf("Granted OAuth scope does not include required %s", GmailReadonlyScope)
	}

	profileReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, gmailProfileURL, nil)
	profileReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	profileRes, err := httpClient.Do(profileReq)
	if err != nil {
		return "", fmt.Errorf("Failed to fetch Gmail profile: %w", err)
	}
	defer profileRes.Body.Close()
	if profileRes.StatusCode < 200 || profileRes.StatusCode >= 300 {
		return "", fmt.Errorf("Failed to fetch Gmail profile: HTTP %d", profileRes.StatusCode)
	}

	var profile struct {
		EmailAddress string `json:"emailAddress"`
	}
	_ = json.NewDecoder(profileRes.Body).Decode(&profile)
	emailAddress := strings.TrimSpace(profile.EmailAddress)
	if emailAddress == "" {
		return "", errors.New("Gmail profile response did not include emailAddress")
	}

	expiresAtUtc := domain.ToUtcIsoSeconds(now.Add(time.Duration(tokenData.ExpiresIn) * time.Second))
	scope := tokenData.Scope
	if scope == "" {
		scope = GmailReadonlyScope
	}

	err = gdb.Transaction(func(tx *gorm.DB) error {
		var existing db.GmailOAuth
		_ = tx.First(&existing, 1).Error
		if existing.EmailAddress != nil && existing.Status != "disconnected" &&
			!strings.EqualFold(*existing.EmailAddress, emailAddress) {
			return fmt.Errorf("Already connected to %s. Please disconnect first before connecting %s.", *existing.EmailAddress, emailAddress)
		}

		nextRefresh := existing.RefreshToken
		if tokenData.RefreshToken != "" {
			rt := tokenData.RefreshToken
			nextRefresh = &rt
		}

		return tx.Model(&db.GmailOAuth{}).Where("id = 1").Updates(map[string]any{
			"status":         "connected",
			"email_address":  emailAddress,
			"access_token":   tokenData.AccessToken,
			"refresh_token":  nextRefresh,
			"expires_at_utc": expiresAtUtc,
			"scope":          scope,
		}).Error
	})
	if err != nil {
		return "", err
	}
	return emailAddress, nil
}

func GetValidAccessToken(ctx context.Context, gdb *gorm.DB, forceRefresh bool) (string, error) {
	var row db.GmailOAuth
	if err := gdb.First(&row, 1).Error; err != nil || row.Status == "disconnected" {
		return "", errors.New("Gmail is not connected")
	}

	now := time.Now().UTC()
	if !forceRefresh && row.Status == "connected" && row.AccessToken != nil && *row.AccessToken != "" && row.ExpiresAtUtc != nil {
		if expMs, err := domain.UtcIsoToEpochMs(*row.ExpiresAtUtc); err == nil {
			if expMs-now.UnixMilli() > accessTokenRefreshBuffer.Milliseconds() {
				return *row.AccessToken, nil
			}
		}
	}

	if row.RefreshToken == nil || *row.RefreshToken == "" {
		gdb.Model(&db.GmailOAuth{}).Where("id = 1").Update("status", "expired")
		return "", &AuthExpiredError{Message: "No Gmail refresh token available; please reconnect Gmail in Settings."}
	}

	clientID, clientSecret, err := getRequiredOAuthClientConfig()
	if err != nil {
		return "", err
	}

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {*row.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Failed to refresh Gmail access token: %w", err)
	}
	defer res.Body.Close()

	var data googleTokenResponse
	_ = json.NewDecoder(res.Body).Decode(&data)
	if res.StatusCode < 200 || res.StatusCode >= 300 || data.AccessToken == "" {
		if res.StatusCode == 400 && data.Error == "invalid_grant" {
			gdb.Model(&db.GmailOAuth{}).Where("id = 1").Update("status", "expired")
			return "", &AuthExpiredError{Message: "Gmail refresh token was revoked or expired (invalid_grant). Please reconnect Gmail in Settings."}
		}
		detail := data.ErrorDescription
		if detail == "" {
			detail = data.Error
		}
		if detail == "" {
			detail = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return "", fmt.Errorf("Failed to refresh Gmail access token: %s", detail)
	}

	expiresIn := data.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	expiresAtUtc := domain.ToUtcIsoSeconds(now.Add(time.Duration(expiresIn) * time.Second))
	nextRefresh := *row.RefreshToken
	if data.RefreshToken != "" {
		nextRefresh = data.RefreshToken
	}
	nextScope := GmailReadonlyScope
	if data.Scope != "" {
		nextScope = data.Scope
	} else if row.Scope != nil && *row.Scope != "" {
		nextScope = *row.Scope
	}

	gdb.Model(&db.GmailOAuth{}).Where("id = 1").Updates(map[string]any{
		"status":         "connected",
		"access_token":   data.AccessToken,
		"refresh_token":  nextRefresh,
		"expires_at_utc": expiresAtUtc,
		"scope":          nextScope,
	})

	return data.AccessToken, nil
}

func DisconnectGmail(ctx context.Context, gdb *gorm.DB) error {
	var row db.GmailOAuth
	_ = gdb.Select("access_token, refresh_token").First(&row, 1).Error
	tokenToRevoke := ""
	if row.RefreshToken != nil && *row.RefreshToken != "" {
		tokenToRevoke = *row.RefreshToken
	} else if row.AccessToken != nil && *row.AccessToken != "" {
		tokenToRevoke = *row.AccessToken
	}

	if err := gdb.Model(&db.GmailOAuth{}).Where("id = 1").Updates(map[string]any{
		"status":         "disconnected",
		"email_address":  nil,
		"access_token":   nil,
		"refresh_token":  nil,
		"expires_at_utc": nil,
		"scope":          nil,
	}).Error; err != nil {
		return err
	}

	if tokenToRevoke != "" {
		httpClient := &http.Client{Timeout: 5 * time.Second}
		form := url.Values{"token": {tokenToRevoke}}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if res, err := httpClient.Do(req); err == nil {
			_ = res.Body.Close()
		}
	}
	return nil
}
