package dashgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ddia/hw1/pkg/golden"
)

const HandlersPath = "/internal/handlers"

func FetchHandlers(ctx context.Context, appURL string, interval time.Duration) (golden.HandlersResponse, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(appURL, "/") + HandlersPath
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		resp, err := fetchOnce(ctx, client, url)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() == nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return golden.HandlersResponse{}, errors.Join(ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func fetchOnce(ctx context.Context, client *http.Client, url string) (golden.HandlersResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return golden.HandlersResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return golden.HandlersResponse{}, fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return golden.HandlersResponse{}, fmt.Errorf("get %s: %s", url, resp.Status)
	}
	var out golden.HandlersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return golden.HandlersResponse{}, fmt.Errorf("decode %s: %w", url, err)
	}
	if len(out.Handlers) == 0 {
		return golden.HandlersResponse{}, fmt.Errorf("%s returned no handlers", url)
	}

	return out, nil
}
