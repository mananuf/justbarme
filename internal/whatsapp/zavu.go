package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// zavuBaseURL and requestTimeout per Zavu's published API reference
// (https://docs.zavu.dev/api-reference/send-a-message). There is no
// published Go SDK to depend on as of this writing -- github.com/zavudev/
// zavu-go does not resolve as a real module -- so this talks to Zavu's
// REST API directly, matching the same request shape their (TypeScript)
// SDK and docs describe. Swap this for an official Go SDK later if/when
// one exists; the Provider interface it implements would not need to
// change.
const (
	zavuBaseURL    = "https://api.zavu.dev"
	requestTimeout = 10 * time.Second
)

type ZavuConfig struct {
	APIKey   string
	SenderID string
}

type ZavuProvider struct {
	cfg        ZavuConfig
	httpClient *http.Client
	baseURL    string
}

func NewZavuProvider(cfg ZavuConfig) *ZavuProvider {
	return &ZavuProvider{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    zavuBaseURL,
	}
}

type sendMessageRequest struct {
	To             string `json:"to"`
	Channel        string `json:"channel"`
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type sendMessageResponse struct {
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
}

type zavuErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (p *ZavuProvider) Send(ctx context.Context, msg Message) (string, error) {
	idempotencyKey := uuid.NewString()
	body, err := json.Marshal(sendMessageRequest{
		To:             msg.To,
		Channel:        "whatsapp",
		Text:           msg.Text,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", fmt.Errorf("marshal zavu request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build zavu request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	if p.cfg.SenderID != "" {
		req.Header.Set("Zavu-Sender", p.cfg.SenderID)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send whatsapp message: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read zavu response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		var zavuErr zavuErrorResponse
		if err := json.Unmarshal(respBody, &zavuErr); err == nil && zavuErr.Error.Message != "" {
			return "", fmt.Errorf("zavu returned %d: %s", resp.StatusCode, zavuErr.Error.Message)
		}
		return "", fmt.Errorf("zavu returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed sendMessageResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse zavu response: %w", err)
	}
	return parsed.Message.ID, nil
}
