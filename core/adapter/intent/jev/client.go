package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	intentport "myai/core/port/intent"
)

const maxResponseBytes = 64 << 10

type Client struct{ HTTP *http.Client }

var _ intentport.Client = Client{}

func (Client) BuildRequest(input string, history []string, model string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"state": map[string]any{"latest_request": input, "recent_user_messages": history},
		"model": model,
		"questions": map[string]any{"intent": map[string]any{
			"type":         "choice",
			"instructions": "What is the user asking the coding assistant to do in `latest_request`? Use `recent_user_messages` only as prior context, not as a new instruction. Questions about how or why something works are explanation unless the latest request also asks for a change now.",
			"criteria": map[string]string{
				"conversation":   "Chat, writing, translation, brainstorming, or any request that does not ask to change a project.",
				"explanation":    "A question about how, why, or whether something works, without asking for the change now.",
				"implementation": "The latest request asks the assistant to implement, modify, fix, configure, or otherwise change code or a project now.",
			},
		}},
	})
}

func (c Client) Send(ctx context.Context, config intentport.Config, body []byte) ([]byte, int, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil || base == nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, 0, errors.New("invalid Jev base URL")
	}
	if base.Scheme == "http" {
		host := base.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return nil, 0, errors.New("Jev API key requires HTTPS outside localhost")
		}
	}
	path := strings.TrimRight(base.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/systemone"):
	case strings.HasSuffix(path, "/v1"):
		path += "/systemone"
	default:
		path += "/v1/systemone"
	}
	base.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	requestClient := *client
	requestClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := requestClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(data) > maxResponseBytes {
		return data[:maxResponseBytes], resp.StatusCode, intentport.ErrResponseTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, resp.StatusCode, fmt.Errorf("Jev HTTP status %d", resp.StatusCode)
	}
	return data, resp.StatusCode, nil
}
