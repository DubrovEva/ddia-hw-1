package integration_test

import (
	"context"
	"io"
	"net/http"

	"ddia/hw2/resilience"
)

var _ resilience.Sender = httpSender{}

type httpSender struct {
	client *http.Client
	url    string
}

func (s httpSender) Send(ctx context.Context) (resilience.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return resilience.Response{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return resilience.Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resilience.Response{}, err
	}

	return resilience.Response{StatusCode: resp.StatusCode, Body: body}, nil
}
