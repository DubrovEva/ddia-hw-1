package resilience_test

import (
	"errors"
	"testing"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func requireOutcome(t *testing.T, err error, want resilience.Outcome) resilience.Response {
	t.Helper()

	var outcomeErr *resilience.OutcomeError
	if !errors.As(err, &outcomeErr) {
		t.Fatalf("err = %v, want OutcomeError with outcome %s", err, want)
	}

	if outcomeErr.Outcome != want {
		t.Fatalf("outcome = %s, want %s", outcomeErr.Outcome, want)
	}

	return outcomeErr.Response
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

func requireCalls(t *testing.T, sender *fake.Sender, want int) {
	t.Helper()

	if got := sender.Calls(); got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}
}
