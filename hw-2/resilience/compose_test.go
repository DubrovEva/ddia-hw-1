package resilience_test

import (
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func outerConfig(budget resilience.RetryBudget) resilience.RetryConfig {
	return resilience.RetryConfig{
		Budget: budget,
		Latency: resilience.LatencyBudget{
			Total:      2 * time.Second,
			Subrequest: time.Second,
		},
	}
}

func perReplicaBackoff() *resilience.Retry {
	return resilience.NewRetry(resilience.RetryConfig{
		Budget: resilience.RetryBudget{Failures: 3, Timeouts: 100},
		Latency: resilience.LatencyBudget{
			Total:      50 * time.Millisecond,
			Subrequest: 10 * time.Millisecond,
		},
		Backoff: resilience.Exponential{Base: 10 * time.Millisecond, Factor: 2},
	})
}

func TestCompose_RoundRobinWithPerReplicaBackoff(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.OK("from b"))
	roundRobin := resilience.NewMultiRetry(outerConfig(resilience.RetryBudget{Failures: 2}), resilience.RoundRobin{})

	start := time.Now()
	resp, err := roundRobin.Do(t.Context(), resilience.PerReplica(perReplicaBackoff(), []resilience.Sender{a, b}))
	elapsed := time.Since(start)

	requireBody(t, resp, err, "from b")
	requireCalls(t, a, 3)
	requireCalls(t, b, 1)

	if want := 30 * time.Millisecond; elapsed < want {
		t.Fatalf("elapsed = %v, want at least %v of backoff on replica a", elapsed, want)
	}
}

func TestCompose_ReplicaRecoversWithinInnerRetries(t *testing.T) {
	a := fake.NewSender(fake.Status(503), fake.OK("from a"))
	b := fake.NewSender(fake.OK("from b"))
	roundRobin := resilience.NewMultiRetry(outerConfig(resilience.RetryBudget{Failures: 2}), resilience.RoundRobin{})

	resp, err := roundRobin.Do(t.Context(), resilience.PerReplica(perReplicaBackoff(), []resilience.Sender{a, b}))

	requireBody(t, resp, err, "from a")
	requireCalls(t, a, 2)
	requireCalls(t, b, 0)
}

func TestCompose_InnerOutcomeIsVisibleToOuterStrategy(t *testing.T) {
	server := fake.NewSender(fake.Status(404))
	outer := resilience.NewRetry(outerConfig(resilience.RetryBudget{FastErrors: 2}))
	inner := resilience.NewRetry(retryConfig(resilience.RetryBudget{FastErrors: 3}))

	_, err := outer.Do(t.Context(), resilience.WithClient(inner, server))

	resp := requireOutcome(t, err, resilience.OutcomeFastError)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404 from the server", resp.StatusCode)
	}

	requireCalls(t, server, 6)
}

func TestCompose_InnerLatencyExhaustionIsTimeoutForOuter(t *testing.T) {
	server := fake.NewSender(fake.Hang())
	outer := resilience.NewRetry(outerConfig(resilience.RetryBudget{Timeouts: 1}))

	_, err := outer.Do(t.Context(), resilience.WithClient(perReplicaBackoff(), server))

	requireOutcome(t, err, resilience.OutcomeTimeout)
}
