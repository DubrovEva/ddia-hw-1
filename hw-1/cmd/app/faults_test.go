package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func callFaults(f faults, ctx context.Context) int {
	rec := httptest.NewRecorder()
	f.wrap(okHandler)(rec, httptest.NewRequest("GET", "/", nil).WithContext(ctx))

	return rec.Code
}

func TestFaultsDisabled(t *testing.T) {
	assert.Equal(t, http.StatusOK, callFaults(faults{}, t.Context()))
}

func TestFaultsAlwaysFail(t *testing.T) {
	assert.Equal(t, http.StatusInternalServerError, callFaults(faults{rate: 1}, t.Context()))
}

func TestFaultsRate(t *testing.T) {
	failed := 0
	for range 2000 {
		if callFaults(faults{rate: 0.3}, t.Context()) == http.StatusInternalServerError {
			failed++
		}
	}
	assert.InDelta(t, 600, failed, 120)
}

func TestFaultsJitterRespectsCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	code := callFaults(faults{jitter: time.Hour}, ctx)

	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Less(t, time.Since(start), time.Second)
}

func TestFaultsFromEnv(t *testing.T) {
	t.Setenv("FAULT_RATE", "0.05")
	t.Setenv("LATENCY_JITTER", "40ms")
	f, err := faultsFromEnv()
	require.NoError(t, err)
	assert.Equal(t, faults{rate: 0.05, jitter: 40 * time.Millisecond}, f)

	t.Setenv("FAULT_RATE", "1.5")
	_, err = faultsFromEnv()
	require.Error(t, err)

	t.Setenv("FAULT_RATE", "0")
	t.Setenv("LATENCY_JITTER", "-1s")
	_, err = faultsFromEnv()
	require.Error(t, err)
}
