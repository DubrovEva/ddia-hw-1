package dashgen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ddia/hw1/pkg/golden"
)

func TestFetchHandlersRetries(t *testing.T) {
	want := golden.HandlersResponse{
		Deployment: "items-service",
		Handlers:   []golden.HandlerInfo{{ID: "items_get", Method: "GET", Path: "/items/{id}"}},
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HandlersPath {
			http.NotFound(w, r)

			return
		}
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	t.Cleanup(srv.Close)

	got, err := FetchHandlers(t.Context(), srv.URL+"/", 5*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, int32(3), calls.Load())
}

func TestFetchHandlersEmptyListTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(golden.HandlersResponse{Deployment: "items-service"})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()

	_, err := FetchHandlers(ctx, srv.URL, 5*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "no handlers")
}
