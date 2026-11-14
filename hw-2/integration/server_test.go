package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"ddia/hw2/resilience"
)

type step func(w http.ResponseWriter, r *http.Request)

func status(code int) step {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

func ok(body string) step {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}
}

func hang() step {
	return func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
}

type server struct {
	srv       *httptest.Server
	mu        sync.Mutex
	steps     []step
	calls     int
	cancelled int
}

func newServer(t *testing.T, steps ...step) *server {
	t.Helper()

	s := &server{steps: steps}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)

	return s
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	step := s.steps[min(s.calls, len(s.steps)-1)]
	s.calls++
	s.mu.Unlock()

	step(w, r)

	if r.Context().Err() != nil {
		s.mu.Lock()
		s.cancelled++
		s.mu.Unlock()
	}
}

func (s *server) sender() resilience.Sender {
	return httpSender{client: s.srv.Client(), url: s.srv.URL}
}

func (s *server) stats() (calls, cancelled int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls, s.cancelled
}

func requireCalls(t *testing.T, s *server, want int) {
	t.Helper()

	if calls, _ := s.stats(); calls != want {
		t.Fatalf("calls = %d, want %d", calls, want)
	}
}

func shutdownAndRequireCancelled(t *testing.T, s *server, want int) {
	t.Helper()

	s.srv.Close()

	if _, cancelled := s.stats(); cancelled != want {
		t.Fatalf("cancelled requests = %d, want %d", cancelled, want)
	}
}

func requireBody(t *testing.T, resp resilience.Response, err error, want string) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(resp.Body) != want {
		t.Fatalf("body = %q, want %q", resp.Body, want)
	}
}
