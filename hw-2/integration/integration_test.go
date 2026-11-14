package integration_test

import (
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
)

func retryConfig(budget resilience.RetryBudget) resilience.RetryConfig {
	return resilience.RetryConfig{
		Budget: budget,
		Latency: resilience.LatencyBudget{
			Total:      2 * time.Second,
			Subrequest: 50 * time.Millisecond,
		},
	}
}

func TestRetry_RecoversFromServerErrors(t *testing.T) {
	srv := newServer(t, status(503), status(503), ok("data"))
	cfg := retryConfig(resilience.RetryBudget{Failures: 3})
	cfg.Backoff = resilience.Exponential{Base: 10 * time.Millisecond, Factor: 2}

	resp, err := resilience.NewRetry(cfg).Do(t.Context(), srv.sender())

	requireBody(t, resp, err, "data")
	requireCalls(t, srv, 3)
}

func TestRetry_CancelsHangingRequests(t *testing.T) {
	srv := newServer(t, hang())

	_, err := resilience.NewRetry(retryConfig(resilience.RetryBudget{Timeouts: 2})).Do(t.Context(), srv.sender())

	var outcomeErr *resilience.OutcomeError
	if !errors.As(err, &outcomeErr) || outcomeErr.Outcome != resilience.OutcomeTimeout {
		t.Fatalf("err = %v, want timeout", err)
	}

	requireCalls(t, srv, 2)
	shutdownAndRequireCancelled(t, srv, 2)
}

func TestRetry_ConnectionErrorIsFailure(t *testing.T) {
	srv := newServer(t, ok("unreachable"))
	sender := srv.sender()
	srv.srv.Close()

	_, err := resilience.NewRetry(retryConfig(resilience.RetryBudget{Failures: 2})).Do(t.Context(), sender)

	var outcomeErr *resilience.OutcomeError
	if !errors.As(err, &outcomeErr) || outcomeErr.Outcome != resilience.OutcomeFailure {
		t.Fatalf("err = %v, want failure", err)
	}
}

func TestRoundRobin_FailsOverToHealthyServer(t *testing.T) {
	broken := newServer(t, status(503))
	healthy := newServer(t, ok("from healthy"))
	client := resilience.NewMultiRetry(retryConfig(resilience.RetryBudget{Failures: 2}), resilience.RoundRobin{})

	resp, err := client.Do(t.Context(), []resilience.Sender{broken.sender(), healthy.sender()})

	requireBody(t, resp, err, "from healthy")
	requireCalls(t, broken, 1)
	requireCalls(t, healthy, 1)
}

func TestHedging_CancelsSlowServer(t *testing.T) {
	slow := newServer(t, hang())
	fast := newServer(t, ok("from fast"))

	resp, err := resilience.NewHedging(20*time.Millisecond, 2*time.Second).
		Do(t.Context(), []resilience.Sender{slow.sender(), fast.sender()})

	requireBody(t, resp, err, "from fast")
	shutdownAndRequireCancelled(t, slow, 1)
}

func TestCircuitBreaker_StopsCallingBrokenServer(t *testing.T) {
	broken := newServer(t, status(503))
	healthy := newServer(t, ok("from healthy"))
	breaker := resilience.NewCircuitBreaker(resilience.BreakerConfig{
		Window:    2,
		Threshold: 0.5,
		Cooldown:  time.Minute,
	})
	client := resilience.NewMultiRetry(retryConfig(resilience.RetryBudget{Failures: 2}), breaker)

	for range 5 {
		resp, err := client.Do(t.Context(), []resilience.Sender{broken.sender(), healthy.sender()})
		requireBody(t, resp, err, "from healthy")
	}

	requireCalls(t, broken, 2)
	requireCalls(t, healthy, 5)
}
