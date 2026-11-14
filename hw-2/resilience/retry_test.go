package resilience_test

import (
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func retryConfig(budget resilience.RetryBudget) resilience.RetryConfig {
	return resilience.RetryConfig{
		Budget: budget,
		Latency: resilience.LatencyBudget{
			Total:      time.Second,
			Subrequest: 20 * time.Millisecond,
		},
	}
}

func TestRetry_RetriesUntilSuccess(t *testing.T) {
	server := fake.NewSender(fake.Status(503), fake.Status(503), fake.OK("data"))
	client := resilience.NewRetry(retryConfig(resilience.RetryBudget{Failures: 3}))

	resp, err := client.Do(t.Context(), server)

	requireBody(t, resp, err, "data")
	requireCalls(t, server, 3)
}

func TestRetry_StopsWhenFailuresBudgetIsExhausted(t *testing.T) {
	server := fake.NewSender(fake.Status(503))
	client := resilience.NewRetry(retryConfig(resilience.RetryBudget{FastErrors: 10, Failures: 3, Timeouts: 10}))

	_, err := client.Do(t.Context(), server)

	requireOutcome(t, err, resilience.OutcomeFailure)
	requireCalls(t, server, 3)
}

func TestRetry_TracksBudgetsPerOutcome(t *testing.T) {
	server := fake.NewSender(fake.Status(404), fake.Status(503), fake.Hang(), fake.Status(404))
	client := resilience.NewRetry(retryConfig(resilience.RetryBudget{FastErrors: 2, Failures: 2, Timeouts: 2}))

	_, err := client.Do(t.Context(), server)

	requireOutcome(t, err, resilience.OutcomeFastError)
	requireCalls(t, server, 4)
}

func TestRetry_StopsWhenTimeoutsBudgetIsExhausted(t *testing.T) {
	server := fake.NewSender(fake.Hang())
	client := resilience.NewRetry(retryConfig(resilience.RetryBudget{Timeouts: 2}))

	_, err := client.Do(t.Context(), server)

	requireOutcome(t, err, resilience.OutcomeTimeout)
	requireCalls(t, server, 2)
}

func TestRetry_ZeroLatencyBudgetMeansNoLimit(t *testing.T) {
	server := fake.NewSender(
		fake.After(10*time.Millisecond, fake.Status(503)),
		fake.After(10*time.Millisecond, fake.OK("data")),
	)
	client := resilience.NewRetry(resilience.RetryConfig{Budget: resilience.RetryBudget{Failures: 2}})

	resp, err := client.Do(t.Context(), server)

	requireBody(t, resp, err, "data")
	requireCalls(t, server, 2)
}

func TestRetry_StopsWhenLatencyBudgetIsExhausted(t *testing.T) {
	server := fake.NewSender(fake.Hang())
	client := resilience.NewRetry(resilience.RetryConfig{
		Budget: resilience.RetryBudget{Timeouts: 100},
		Latency: resilience.LatencyBudget{
			Total:      50 * time.Millisecond,
			Subrequest: 30 * time.Millisecond,
		},
	})

	_, err := client.Do(t.Context(), server)

	if !errors.Is(err, resilience.ErrLatencyExhausted) {
		t.Fatalf("err = %v, want ErrLatencyExhausted", err)
	}

	requireCalls(t, server, 2)
}
