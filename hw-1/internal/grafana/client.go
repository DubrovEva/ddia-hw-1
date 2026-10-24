package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
	auth    func(*http.Request)
}

type ServiceAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type Token struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

func NewBasicAuth(baseURL, user, password string) *Client {
	return newClient(baseURL, func(r *http.Request) { r.SetBasicAuth(user, password) })
}

func NewTokenAuth(baseURL, token string) *Client {
	return newClient(baseURL, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) })
}

func newClient(baseURL string, auth func(*http.Request)) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
		auth:    auth,
	}
}

func (c *Client) Health(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/health", nil, nil)
}

func (c *Client) FindServiceAccount(ctx context.Context, name string) (ServiceAccount, bool, error) {
	var resp struct {
		ServiceAccounts []ServiceAccount `json:"serviceAccounts"`
	}
	path := "/api/serviceaccounts/search?query=" + url.QueryEscape(name)
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return ServiceAccount{}, false, err
	}
	for _, sa := range resp.ServiceAccounts {
		if sa.Name == name {
			return sa, true, nil
		}
	}

	return ServiceAccount{}, false, nil
}

func (c *Client) CreateServiceAccount(ctx context.Context, name, role string) (ServiceAccount, error) {
	req := map[string]any{"name": name, "role": role, "isDisabled": false}
	var sa ServiceAccount
	err := c.do(ctx, http.MethodPost, "/api/serviceaccounts", req, &sa)

	return sa, err
}

func (c *Client) ListTokens(ctx context.Context, saID int64) ([]Token, error) {
	var tokens []Token
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/serviceaccounts/%d/tokens", saID), nil, &tokens)

	return tokens, err
}

func (c *Client) DeleteToken(ctx context.Context, saID, tokenID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/serviceaccounts/%d/tokens/%d", saID, tokenID), nil, nil)
}

func (c *Client) CreateToken(ctx context.Context, saID int64, name string) (Token, error) {
	var token Token
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/serviceaccounts/%d/tokens", saID), map[string]any{"name": name}, &token)

	return token, err
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("grafana: encode %s %s: %w", method, path, err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("grafana: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.auth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("grafana: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

		return fmt.Errorf("grafana: %s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("grafana: decode %s %s: %w", method, path, err)
	}

	return nil
}
