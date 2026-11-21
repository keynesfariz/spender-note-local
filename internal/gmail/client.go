package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"portspendernote/internal/domain"
)

const (
	gmailMessagesURL        = "https://gmail.googleapis.com/gmail/v1/users/me/messages"
	defaultConcurrency      = 4
	defaultMaxRetries       = 3
	defaultBaseRetryDelayMs = 250
	maxRetryAfterMs         = 60_000
	requestTimeout          = 15 * time.Second
)

var (
	quotedRe   = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
	commentPRe = regexp.MustCompile(`\([^()]*\)`)
	angleRe    = regexp.MustCompile(`<([^<>\s@]+@[^<>\s@]+)>`)
	bareAddrRe = regexp.MustCompile(`^([^\s<>@,;:"()]+@[^\s<>@,;:"()]+)$`)
)

type UnauthorizedError struct{}

func (UnauthorizedError) Error() string { return "Gmail API returned HTTP 401 Unauthorized" }

type FetchedMessage struct {
	GmailMessageID string
	FromAddress    string
	Subject        string
	CleanedText    string
	InternalDateMs int64
	ReceivedAtUtc  string
}

type FetchSenderMessagesResult struct {
	Messages []FetchedMessage
	HasMore  bool
}

func ExtractFromAddrSpec(fromHeader string) string {
	if fromHeader == "" {
		return ""
	}
	s := quotedRe.ReplaceAllString(fromHeader, " ")
	s = strings.TrimSpace(commentPRe.ReplaceAllString(s, " "))
	candidate := ""
	if m := angleRe.FindStringSubmatch(s); m != nil {
		candidate = m[1]
	} else if m := bareAddrRe.FindStringSubmatch(s); m != nil {
		candidate = m[1]
	}
	if candidate == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(candidate))
	atIdx := strings.IndexByte(lower, '@')
	if atIdx <= 0 || atIdx == len(lower)-1 {
		return ""
	}
	if !strings.Contains(lower[atIdx+1:], ".") {
		return ""
	}
	return lower
}

func getHeaderValue(headers []MessageHeader, target string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, target) {
			return h.Value
		}
	}
	return ""
}

func isRetryableRateLimit(status int, body []byte) bool {
	if status == 429 {
		return true
	}
	if status != 403 {
		return false
	}
	var parsed struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	for _, e := range parsed.Error.Errors {
		if e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" {
			return true
		}
	}
	return false
}

func fetchWithRateLimitRetry(ctx context.Context, client *http.Client, targetURL, accessToken string) (int, []byte, error) {
	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
		if err != nil {
			cancel()
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		res, err := client.Do(req)
		if err != nil {
			cancel()
			return 0, nil, err
		}
		body, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		cancel()
		if readErr != nil {
			return res.StatusCode, nil, readErr
		}

		if attempt == defaultMaxRetries || !isRetryableRateLimit(res.StatusCode, body) {
			return res.StatusCode, body, nil
		}

		delayMs := defaultBaseRetryDelayMs * (1 << attempt)
		if retryAfter := res.Header.Get("Retry-After"); retryAfter != "" {
			if sec, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && sec >= 0 {
				delayMs = sec * 1000
			}
		}
		if delayMs > maxRetryAfterMs {
			delayMs = maxRetryAfterMs
		}
		delayMs += rand.IntN(250)

		timer := time.NewTimer(time.Duration(delayMs) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, nil, ctx.Err()
		case <-timer.C:
		}
	}
	return 0, nil, errors.New("Exhausted rate-limit retries")
}

type FetchSenderMessagesParams struct {
	AccessToken     string
	SenderEmail     string
	AfterEpochSec   int64
	KnownMessageIDs map[string]struct{}
	MaxMessages     int
	OnMessage       func(FetchedMessage)
}

func FetchSenderMessages(ctx context.Context, params FetchSenderMessagesParams) (*FetchSenderMessagesResult, error) {
	if params.MaxMessages == 0 {
		return &FetchSenderMessagesResult{Messages: []FetchedMessage{}, HasMore: false}, nil
	}
	expectedEmail := strings.ToLower(strings.TrimSpace(params.SenderEmail))
	query := fmt.Sprintf(`from:"%s" after:%d`, expectedEmail, params.AfterEpochSec)
	client := &http.Client{}

	var messageIDs []string
	seenIDs := map[string]struct{}{}
	pageToken := ""
	hasMore := false

	for {
		u, _ := url.Parse(gmailMessagesURL)
		q := u.Query()
		q.Set("q", query)
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		u.RawQuery = q.Encode()

		status, body, err := fetchWithRateLimitRetry(ctx, client, u.String(), params.AccessToken)
		if err != nil {
			return nil, err
		}
		if status == 401 {
			return nil, UnauthorizedError{}
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("Gmail messages.list failed: HTTP %d", status)
		}

		var listResp struct {
			Messages      []struct{ ID string `json:"id"` } `json:"messages"`
			NextPageToken string                            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, err
		}

		reachedCap := false
		for _, m := range listResp.Messages {
			if m.ID == "" {
				continue
			}
			if _, known := params.KnownMessageIDs[m.ID]; known {
				continue
			}
			if _, seen := seenIDs[m.ID]; seen {
				continue
			}
			if params.MaxMessages > 0 && len(messageIDs) >= params.MaxMessages {
				hasMore = true
				reachedCap = true
				break
			}
			seenIDs[m.ID] = struct{}{}
			messageIDs = append(messageIDs, m.ID)
		}

		pageToken = listResp.NextPageToken
		if reachedCap {
			break
		}
		if params.MaxMessages > 0 && len(messageIDs) >= params.MaxMessages {
			if pageToken != "" {
				hasMore = true
			}
			break
		}
		if pageToken == "" {
			break
		}
	}

	if len(messageIDs) == 0 {
		return &FetchSenderMessagesResult{Messages: []FetchedMessage{}, HasMore: hasMore}, nil
	}

	results := make([]*FetchedMessage, len(messageIDs))
	var firstErr error
	var errOnce sync.Once
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int, len(messageIDs))
	for i := range messageIDs {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	workers := min(defaultConcurrency, len(messageIDs))
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if workCtx.Err() != nil {
					return
				}
				id := messageIDs[idx]
				getURL := fmt.Sprintf("%s/%s?format=full", gmailMessagesURL, url.PathEscape(id))
				status, body, err := fetchWithRateLimitRetry(workCtx, client, getURL, params.AccessToken)
				if err != nil {
					errOnce.Do(func() { firstErr = err; cancel() })
					return
				}
				if status == 401 {
					errOnce.Do(func() { firstErr = UnauthorizedError{}; cancel() })
					return
				}
				if status < 200 || status >= 300 {
					errOnce.Do(func() { firstErr = fmt.Errorf("Gmail messages.get(%s) failed: HTTP %d", id, status); cancel() })
					return
				}

				var msg struct {
					ID           string       `json:"id"`
					InternalDate string       `json:"internalDate"`
					Payload      *MessagePart `json:"payload"`
				}
				if err := json.Unmarshal(body, &msg); err != nil {
					errOnce.Do(func() { firstErr = err; cancel() })
					return
				}
				var headers []MessageHeader
				if msg.Payload != nil {
					headers = msg.Payload.Headers
				}
				addrSpec := ExtractFromAddrSpec(getHeaderValue(headers, "From"))
				if addrSpec == "" || addrSpec != expectedEmail {
					continue
				}
				internalDateMs, err := strconv.ParseInt(msg.InternalDate, 10, 64)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("Gmail message %s missing valid internalDate", id); cancel() })
					return
				}
				parsed := FetchedMessage{
					GmailMessageID: msg.ID,
					FromAddress:    addrSpec,
					Subject:        getHeaderValue(headers, "Subject"),
					CleanedText:    ExtractEmailText(msg.Payload, DefaultMaxTextLength),
					InternalDateMs: internalDateMs,
					ReceivedAtUtc:  domain.ToUtcIsoSeconds(time.UnixMilli(internalDateMs)),
				}
				results[idx] = &parsed
				if params.OnMessage != nil {
					params.OnMessage(parsed)
				}
			}
		}()
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	messages := make([]FetchedMessage, 0, len(results))
	for _, r := range results {
		if r != nil {
			messages = append(messages, *r)
		}
	}
	return &FetchSenderMessagesResult{Messages: messages, HasMore: hasMore}, nil
}
