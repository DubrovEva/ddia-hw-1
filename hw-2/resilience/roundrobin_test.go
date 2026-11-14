package resilience_test

import (
	"errors"
	"testing"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func roundRobin(budget resilience.RetryBudget) *resilience.MultiRetry {
	return resilience.NewMultiRetry(retryConfig(budget), resilience.RoundRobin{})
}

func TestRoundRobin_MovesToNextReplicaOnError(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(503))
	c := fake.NewSender(fake.OK("from c"))

	resp, err := roundRobin(resilience.RetryBudget{Failures: 3}).Do(t.Context(), []resilience.Sender{a, b, c})

	requireBody(t, resp, err, "from c")
	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
	requireCalls(t, c, 1)
}

func TestRoundRobin_WrapsAroundToFirstReplica(t *testing.T) {
	a := fake.NewSender(fake.Status(503), fake.OK("from a"))
	b := fake.NewSender(fake.Status(503))
	c := fake.NewSender(fake.Status(503))

	resp, err := roundRobin(resilience.RetryBudget{Failures: 4}).Do(t.Context(), []resilience.Sender{a, b, c})

	requireBody(t, resp, err, "from a")
	requireCalls(t, a, 2)
	requireCalls(t, b, 1)
	requireCalls(t, c, 1)
}

func TestRoundRobin_SkipsHangingReplicaAfterTimeout(t *testing.T) {
	a := fake.NewSender(fake.Hang())
	b := fake.NewSender(fake.OK("from b"))

	resp, err := roundRobin(resilience.RetryBudget{Timeouts: 2}).Do(t.Context(), []resilience.Sender{a, b})

	requireBody(t, resp, err, "from b")
	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
}

func TestRoundRobin_BudgetIsSharedAcrossReplicas(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(503))
	c := fake.NewSender(fake.Status(503))

	_, err := roundRobin(resilience.RetryBudget{Failures: 4}).Do(t.Context(), []resilience.Sender{a, b, c})

	requireOutcome(t, err, resilience.OutcomeFailure)
	requireCalls(t, a, 2)
	requireCalls(t, b, 1)
	requireCalls(t, c, 1)
}

func TestRoundRobin_IsDefaultBalancer(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.OK("from b"))
	client := resilience.NewMultiRetry(retryConfig(resilience.RetryBudget{Failures: 2}), nil)

	resp, err := client.Do(t.Context(), []resilience.Sender{a, b})

	requireBody(t, resp, err, "from b")
	requireCalls(t, a, 1)
}

func TestRoundRobin_NoReplicas(t *testing.T) {
	_, err := roundRobin(resilience.RetryBudget{Failures: 1}).Do(t.Context(), nil)

	if !errors.Is(err, resilience.ErrNoReplicas) {
		t.Fatalf("err = %v, want ErrNoReplicas", err)
	}
}
