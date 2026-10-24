package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGrafana struct {
	mu       sync.Mutex
	healthy  bool
	accounts []ServiceAccount
	tokens   map[int64][]Token
	nextID   int64
}

func newFakeGrafana(t *testing.T) (*fakeGrafana, *httptest.Server) {
	t.Helper()

	f := &fakeGrafana{healthy: true, tokens: map[int64][]Token{}, nextID: 1}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", f.health)
	mux.HandleFunc("GET /api/serviceaccounts/search", f.search)
	mux.HandleFunc("POST /api/serviceaccounts", f.createAccount)
	mux.HandleFunc("GET /api/serviceaccounts/{id}/tokens", f.listTokens)
	mux.HandleFunc("POST /api/serviceaccounts/{id}/tokens", f.createToken)
	mux.HandleFunc("DELETE /api/serviceaccounts/{id}/tokens/{tid}", f.deleteToken)

	srv := httptest.NewServer(requireBasicAuth(mux))
	t.Cleanup(srv.Close)

	return f, srv
}

func requireBasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "secret" {
			http.Error(w, `{"message":"invalid credentials"}`, http.StatusUnauthorized)

			return
		}
		next.ServeHTTP(w, r)
	})
}

func (f *fakeGrafana) health(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.healthy {
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}
	writeTestJSON(w, map[string]string{"database": "ok"})
}

func (f *fakeGrafana) search(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	writeTestJSON(w, map[string]any{"serviceAccounts": f.accounts})
}

func (f *fakeGrafana) createAccount(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var req ServiceAccount
	_ = json.NewDecoder(r.Body).Decode(&req)
	sa := ServiceAccount{ID: f.nextID, Name: req.Name, Role: req.Role}
	f.nextID++
	f.accounts = append(f.accounts, sa)
	writeTestJSON(w, sa)
}

func (f *fakeGrafana) listTokens(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tokens := []Token{}
	for _, t := range f.tokens[id] {
		tokens = append(tokens, Token{ID: t.ID, Name: t.Name})
	}
	writeTestJSON(w, tokens)
}

func (f *fakeGrafana) createToken(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req Token
	_ = json.NewDecoder(r.Body).Decode(&req)
	t := Token{ID: f.nextID, Name: req.Name, Key: fmt.Sprintf("glsa_%d", f.nextID)}
	f.nextID++
	f.tokens[id] = append(f.tokens[id], t)
	writeTestJSON(w, t)
}

func (f *fakeGrafana) deleteToken(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tid, _ := strconv.ParseInt(r.PathValue("tid"), 10, 64)
	kept := f.tokens[id][:0]
	for _, t := range f.tokens[id] {
		if t.ID != tid {
			kept = append(kept, t)
		}
	}
	f.tokens[id] = kept
	writeTestJSON(w, map[string]string{"message": "deleted"})
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestRotateServiceAccountTokenCreatesAccount(t *testing.T) {
	f, srv := newFakeGrafana(t)
	c := NewBasicAuth(srv.URL, "admin", "secret")

	token, err := c.RotateServiceAccountToken(t.Context(), "dashgen", "Editor")
	require.NoError(t, err)
	assert.NotEmpty(t, token.Key)

	require.Len(t, f.accounts, 1)
	assert.Equal(t, "dashgen", f.accounts[0].Name)
	assert.Equal(t, "Editor", f.accounts[0].Role)
	assert.Len(t, f.tokens[f.accounts[0].ID], 1)
}

func TestRotateServiceAccountTokenIsIdempotent(t *testing.T) {
	f, srv := newFakeGrafana(t)
	c := NewBasicAuth(srv.URL, "admin", "secret")

	first, err := c.RotateServiceAccountToken(t.Context(), "dashgen", "Editor")
	require.NoError(t, err)
	second, err := c.RotateServiceAccountToken(t.Context(), "dashgen", "Editor")
	require.NoError(t, err)

	assert.NotEqual(t, first.Key, second.Key)
	require.Len(t, f.accounts, 1)
	tokens := f.tokens[f.accounts[0].ID]
	require.Len(t, tokens, 1)
	assert.Equal(t, second.ID, tokens[0].ID)
}

func TestRotateServiceAccountTokenIgnoresSimilarNames(t *testing.T) {
	f, srv := newFakeGrafana(t)
	f.accounts = []ServiceAccount{{ID: 100, Name: "dashgen-old", Role: "Viewer"}}
	c := NewBasicAuth(srv.URL, "admin", "secret")

	_, err := c.RotateServiceAccountToken(t.Context(), "dashgen", "Editor")
	require.NoError(t, err)

	require.Len(t, f.accounts, 2)
	assert.Empty(t, f.tokens[100])
}

func TestClientReportsHTTPErrors(t *testing.T) {
	_, srv := newFakeGrafana(t)
	c := NewBasicAuth(srv.URL, "admin", "wrong")

	_, err := c.RotateServiceAccountToken(t.Context(), "dashgen", "Editor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "invalid credentials")
}

func TestWaitHealthy(t *testing.T) {
	f, srv := newFakeGrafana(t)
	f.healthy = false
	c := NewBasicAuth(srv.URL, "admin", "secret")

	go func() {
		time.Sleep(30 * time.Millisecond)
		f.mu.Lock()
		f.healthy = true
		f.mu.Unlock()
	}()
	require.NoError(t, c.WaitHealthy(t.Context(), 10*time.Millisecond))

	f.mu.Lock()
	f.healthy = false
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, c.WaitHealthy(ctx, 10*time.Millisecond), context.DeadlineExceeded)
}
