package resilience_test

import (
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func TestExponential_GrowsUpToMax(t *testing.T) {
	backoff := resilience.Exponential{Base: 10 * time.Millisecond, Factor: 2, Max: 50 * time.Millisecond}
	want := []time.Duration{10, 20, 40, 50, 50}

	for i, w := range want {
		if got := backoff.Delay(i + 1); got != w*time.Millisecond {
			t.Errorf("Delay(%d) = %v, want %v", i+1, got, w*time.Millisecond)
		}
	}
}

func TestExponentialRetry_WaitsAfterServerErrors(t *testing.T) {
	server := fake.NewSender(fake.Status(503), fake.Status(503), fake.OK("data"))
	cfg := retryConfig(resilience.RetryBudget{Failures: 3})
	cfg.Backoff = resilience.Exponential{Base: 20 * time.Millisecond, Factor: 2}

	start := time.Now()
	resp, err := resilience.NewRetry(cfg).Do(t.Context(), server)
	elapsed := time.Since(start)

	requireBody(t, resp, err, "data")

	if want := 60 * time.Millisecond; elapsed < want {
		t.Fatalf("elapsed = %v, want at least %v", elapsed, want)
	}
}

func TestExponentialRetry_WaitsAfterTimeouts(t *testing.T) {
	server := fake.NewSender(fake.Hang(), fake.OK("data"))
	cfg := retryConfig(resilience.RetryBudget{Timeouts: 2})
	cfg.Backoff = resilience.Exponential{Base: 50 * time.Millisecond, Factor: 2}

	start := time.Now()
	resp, err := resilience.NewRetry(cfg).Do(t.Context(), server)
	elapsed := time.Since(start)

	requireBody(t, resp, err, "data")

	if want := cfg.Latency.Subrequest + 50*time.Millisecond; elapsed < want {
		t.Fatalf("elapsed = %v, want at least %v", elapsed, want)
	}
}

func TestExponentialRetry_DoesNotWaitAfterClientErrors(t *testing.T) {
	server := fake.NewSender(fake.Status(404), fake.Status(404), fake.OK("data"))
	cfg := retryConfig(resilience.RetryBudget{FastErrors: 3})
	cfg.Backoff = resilience.Exponential{Base: 200 * time.Millisecond, Factor: 2}

	start := time.Now()
	resp, err := resilience.NewRetry(cfg).Do(t.Context(), server)
	elapsed := time.Since(start)

	requireBody(t, resp, err, "data")

	if limit := 100 * time.Millisecond; elapsed > limit {
		t.Fatalf("elapsed = %v, want under %v", elapsed, limit)
	}
}

func TestExponentialRetry_DelayIsBoundedByLatencyBudget(t *testing.T) {
	server := fake.NewSender(fake.Status(503))
	cfg := retryConfig(resilience.RetryBudget{Failures: 10})
	cfg.Latency.Total = 50 * time.Millisecond
	cfg.Backoff = resilience.Exponential{Base: time.Second, Factor: 2}

	start := time.Now()
	_, err := resilience.NewRetry(cfg).Do(t.Context(), server)
	elapsed := time.Since(start)

	if !errors.Is(err, resilience.ErrLatencyExhausted) {
		t.Fatalf("err = %v, want ErrLatencyExhausted", err)
	}

	requireCalls(t, server, 1)

	if limit := 500 * time.Millisecond; elapsed > limit {
		t.Fatalf("elapsed = %v, want under %v", elapsed, limit)
	}
}
