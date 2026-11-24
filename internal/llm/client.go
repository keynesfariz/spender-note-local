package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const DefaultBaseURL = "http://localhost:9931/v1"

func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultBaseURL, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("Invalid LLM server URL: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("LLM server URL must start with http:// or https://.")
	}
	if parsed.Host == "" {
		return "", errors.New("LLM server URL must include a host (for example http://localhost:9931/v1).")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

type Client struct {
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		BaseURL:    normalized,
		Timeout:    timeout,
		HTTPClient: &http.Client{},
	}, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, &RuntimeError{Message: fmt.Sprintf("Failed to build models request: %v", err), Cause: err}
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &RuntimeError{
			Message: fmt.Sprintf("Cannot reach local LLM server at %s. Make sure llama.app (or your configured server) is running.", c.BaseURL),
			Cause:   err,
		}
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &RuntimeError{Message: fmt.Sprintf("Failed reading models response: %v", err), Cause: err}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &RuntimeError{Message: fmt.Sprintf("Local LLM server returned HTTP %d on /models", res.StatusCode)}
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &RuntimeError{Message: fmt.Sprintf("Invalid /models response from %s: %v", c.BaseURL, err), Cause: err}
	}

	seen := map[string]struct{}{}
	models := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models, nil
}

func (c *Client) Preflight(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		return &RuntimeError{Message: "Select a model from your local LLM server in Settings before syncing."}
	}
	checkCtx, cancel := context.WithTimeout(ctx, min(c.Timeout, 5*time.Second))
	defer cancel()
	if _, err := c.ListModels(checkCtx); err != nil {
		return err
	}
	return nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	CachePrompt    bool           `json:"cache_prompt,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) doChatCompletion(ctx context.Context, payload chatCompletionRequest) (int, string, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, "", &RuntimeError{Message: fmt.Sprintf("Failed to encode LLM request: %v", err), Cause: err}
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, "", &RuntimeError{Message: fmt.Sprintf("Failed to build LLM request: %v", err), Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, "", &RuntimeError{Message: "LLM extraction cancelled", Cause: ctx.Err()}
		}
		if reqCtx.Err() != nil {
			return 0, "", &ExtractionError{Message: fmt.Sprintf("LLM extraction timed out after %dms", c.Timeout.Milliseconds()), Cause: reqCtx.Err()}
		}
		return 0, "", &RuntimeError{
			Message: fmt.Sprintf("Local LLM server at %s became unreachable: %v", c.BaseURL, err),
			Cause:   err,
		}
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		if ctx.Err() != nil {
			return res.StatusCode, "", &RuntimeError{Message: "LLM extraction cancelled", Cause: ctx.Err()}
		}
		if reqCtx.Err() != nil {
			return res.StatusCode, "", &ExtractionError{Message: fmt.Sprintf("LLM extraction timed out after %dms", c.Timeout.Milliseconds()), Cause: reqCtx.Err()}
		}
		return res.StatusCode, "", &RuntimeError{Message: fmt.Sprintf("Failed reading LLM response: %v", err), Cause: err}
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return res.StatusCode, "", &RuntimeError{Message: fmt.Sprintf("Local LLM server returned HTTP %d", res.StatusCode)}
		}
		return res.StatusCode, "", &RuntimeError{Message: fmt.Sprintf("Invalid chat completion JSON (HTTP %d)", res.StatusCode), Cause: err}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || parsed.Error != nil {
		msg := fmt.Sprintf("Local LLM server returned HTTP %d", res.StatusCode)
		if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			msg = parsed.Error.Message
		}
		return res.StatusCode, "", &RuntimeError{Message: msg}
	}
	if len(parsed.Choices) == 0 {
		return res.StatusCode, "", &ExtractionError{Message: "Local LLM server returned no choices"}
	}
	return res.StatusCode, parsed.Choices[0].Message.Content, nil
}

func (c *Client) CheckHealth(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return &RuntimeError{Message: "No model selected."}
	}
	listCtx, cancel := context.WithTimeout(ctx, min(c.Timeout, 5*time.Second))
	defer cancel()
	if _, err := c.ListModels(listCtx); err != nil {
		return err
	}
	_, _, err := c.doChatCompletion(ctx, chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: "Reply OK"},
		},
		Temperature: 0,
		MaxTokens:   8,
	})
	return err
}

func (c *Client) completeStructuredJSON(ctx context.Context, model, emptyModelErr, schemaName string, schema map[string]any, systemPrompt, userPrompt string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", &RuntimeError{Message: emptyModelErr}
	}

	req := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
		CachePrompt: true,
		ResponseFormat: map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   schemaName,
				"strict": true,
				"schema": schema,
			},
		},
	}

	status, content, err := c.doChatCompletion(ctx, req)
	if err != nil && status == http.StatusBadRequest {
		// Fallback for OpenAI-compatible servers that reject json_schema constraints.
		req.ResponseFormat = map[string]any{"type": "json_object"}
		_, content, err = c.doChatCompletion(ctx, req)
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

func (c *Client) ExtractTransaction(ctx context.Context, model string, input ExtractInput) (*ExtractResult, error) {
	categories := ResolveCategories(input.Categories)
	accountIDs := make([]string, 0, len(input.Accounts))
	for _, a := range input.Accounts {
		accountIDs = append(accountIDs, a.ID)
	}
	systemPrompt, userPrompt := BuildExtractionPrompt(input)
	schema := BuildExtractionJSONSchema(categories, accountIDs)

	content, err := c.completeStructuredJSON(
		ctx,
		model,
		"No model configured for extraction.",
		"transaction_extraction",
		schema,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return nil, err
	}

	raw, cleanedJSON, err := ParseAndValidateExtraction(content, categories)
	if err != nil {
		return nil, err
	}
	return NormalizeExtractedResult(raw, cleanedJSON, input)
}

func (c *Client) EnrichTransaction(ctx context.Context, model string, input EnrichInput) (*EnrichResult, error) {
	categories := ResolveCategories(input.Categories)
	systemPrompt, userPrompt := BuildEnrichmentPrompt(input)
	schema := BuildEnrichmentJSONSchema(categories)

	content, err := c.completeStructuredJSON(
		ctx,
		model,
		"No model configured for enrichment.",
		"transaction_enrichment",
		schema,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return nil, err
	}

	raw, cleanedJSON, err := ParseAndValidateEnrichment(content)
	if err != nil {
		return nil, err
	}
	return NormalizeEnrichedResult(raw, cleanedJSON, input)
}
