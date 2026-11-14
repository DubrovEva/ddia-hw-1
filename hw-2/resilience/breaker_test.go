package resilience_test

import (
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

const cooldown = 100 * time.Millisecond

func newBreaker(window int, threshold float64) *resilience.CircuitBreaker {
	return resilience.NewCircuitBreaker(resilience.BreakerConfig{
		Window:    window,
		Threshold: threshold,
		Cooldown:  cooldown,
	})
}

func withBreaker(budget resilience.RetryBudget, window int, threshold float64) *resilience.MultiRetry {
	return resilience.NewMultiRetry(retryConfig(budget), newBreaker(window, threshold))
}

func doTimes(t *testing.T, client resilience.MultiClient, replicas []resilience.Sender, times int) {
	t.Helper()

	for range times {
		if _, err := client.Do(t.Context(), replicas); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestBreaker_BehavesLikeRoundRobinWhileHealthy(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(503))
	c := fake.NewSender(fake.OK("from c"))
	client := withBreaker(resilience.RetryBudget{Failures: 3}, 10, 0.5)

	resp, err := client.Do(t.Context(), []resilience.Sender{a, b, c})

	requireBody(t, resp, err, "from c")
	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
	requireCalls(t, c, 1)
}

func TestBreaker_FailingReplicaIsRemovedFromRotation(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.OK("from b"))
	client := withBreaker(resilience.RetryBudget{Failures: 2}, 2, 0.5)

	doTimes(t, client, []resilience.Sender{a, b}, 5)

	requireCalls(t, a, 2)
	requireCalls(t, b, 5)
}

func TestBreaker_ReplicaReturnsAfterCooldown(t *testing.T) {
	a := fake.NewSender(fake.Status(503), fake.Status(503), fake.OK("from a"))
	b := fake.NewSender(fake.OK("from b"))
	client := withBreaker(resilience.RetryBudget{Failures: 2}, 2, 0.5)
	replicas := []resilience.Sender{a, b}

	doTimes(t, client, replicas, 3)
	requireCalls(t, a, 2)

	time.Sleep(cooldown + 20*time.Millisecond)

	resp, err := client.Do(t.Context(), replicas)

	requireBody(t, resp, err, "from a")
	requireCalls(t, a, 3)
}

func TestBreaker_OccasionalErrorsDoNotOpenCircuit(t *testing.T) {
	a := fake.NewSender(
		fake.Status(503), fake.OK("from a"),
		fake.Status(503), fake.OK("from a"),
		fake.Status(503), fake.OK("from a"),
	)
	b := fake.NewSender(fake.OK("from b"))
	client := withBreaker(resilience.RetryBudget{Failures: 2}, 4, 0.5)

	doTimes(t, client, []resilience.Sender{a, b}, 6)

	requireCalls(t, a, 6)
	requireCalls(t, b, 3)
}

func TestBreaker_AllReplicasOpen(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(503))
	client := withBreaker(resilience.RetryBudget{Failures: 10}, 1, 0)

	_, err := client.Do(t.Context(), []resilience.Sender{a, b})

	if !errors.Is(err, resilience.ErrNoReplicas) {
		t.Fatalf("err = %v, want ErrNoReplicas", err)
	}

	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
}

func TestBreaker_TriesEveryReplicaWhenOneOpensMidRequest(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(503))
	c := fake.NewSender(fake.OK("from c"))
	breaker := newBreaker(2, 0.5)
	breaker.Report(1, resilience.OutcomeFailure)
	client := resilience.NewMultiRetry(retryConfig(resilience.RetryBudget{Failures: 3}), breaker)

	resp, err := client.Do(t.Context(), []resilience.Sender{a, b, c})

	requireBody(t, resp, err, "from c")
	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
}

func TestBreaker_DoesNotWaitWhenNoReplicasLeft(t *testing.T) {
	server := fake.NewSender(fake.Status(503))
	cfg := retryConfig(resilience.RetryBudget{Failures: 10})
	cfg.Backoff = resilience.Exponential{Base: time.Second, Factor: 2}
	client := resilience.NewMultiRetry(cfg, newBreaker(1, 0))

	start := time.Now()
	_, err := client.Do(t.Context(), []resilience.Sender{server})
	elapsed := time.Since(start)

	if !errors.Is(err, resilience.ErrNoReplicas) {
		t.Fatalf("err = %v, want ErrNoReplicas", err)
	}

	if resp := requireOutcome(t, err, resilience.OutcomeFailure); resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	if limit := 500 * time.Millisecond; elapsed > limit {
		t.Fatalf("elapsed = %v, want under %v", elapsed, limit)
	}
}

func TestBreaker_LateReportsDoNotExtendCooldown(t *testing.T) {
	breaker := newBreaker(1, 0)
	breaker.Report(0, resilience.OutcomeFailure)

	if _, err := breaker.Pick(0, 1); !errors.Is(err, resilience.ErrNoReplicas) {
		t.Fatalf("err = %v, want ErrNoReplicas while open", err)
	}

	time.Sleep(cooldown / 2)
	breaker.Report(0, resilience.OutcomeFailure)
	time.Sleep(cooldown/2 + 20*time.Millisecond)

	if _, err := breaker.Pick(0, 1); err != nil {
		t.Fatalf("err = %v, want replica back after cooldown", err)
	}
}

func TestBreaker_RejectsEmptyWindow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic for zero window")
		}
	}()

	resilience.NewCircuitBreaker(resilience.BreakerConfig{Threshold: 0.5, Cooldown: cooldown})
}
