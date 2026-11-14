package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func TestDefault_Success(t *testing.T) {
	server := fake.NewSender(fake.OK("data"))

	resp, err := resilience.NewDefault(time.Second).Do(t.Context(), server)

	requireBody(t, resp, err, "data")
	requireCalls(t, server, 1)
}

func TestDefault_ServerErrorIsNotRetried(t *testing.T) {
	server := fake.NewSender(fake.Status(503), fake.OK("data"))

	_, err := resilience.NewDefault(time.Second).Do(t.Context(), server)

	resp := requireOutcome(t, err, resilience.OutcomeFailure)
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	requireCalls(t, server, 1)
}

func TestDefault_ClientError(t *testing.T) {
	server := fake.NewSender(fake.Status(404))

	_, err := resilience.NewDefault(time.Second).Do(t.Context(), server)

	requireOutcome(t, err, resilience.OutcomeFastError)
	requireCalls(t, server, 1)
}

func TestDefault_HangingServerTimesOut(t *testing.T) {
	const timeout = 20 * time.Millisecond

	server := fake.NewSender(fake.Hang())

	start := time.Now()
	_, err := resilience.NewDefault(timeout).Do(t.Context(), server)
	elapsed := time.Since(start)

	requireOutcome(t, err, resilience.OutcomeTimeout)

	if elapsed < timeout || elapsed > 10*timeout {
		t.Fatalf("elapsed = %v, want about %v", elapsed, timeout)
	}
}

func TestDefault_ZeroTimeoutMeansNoLimit(t *testing.T) {
	server := fake.NewSender(fake.After(20*time.Millisecond, fake.OK("data")))

	resp, err := resilience.NewDefault(0).Do(t.Context(), server)

	requireBody(t, resp, err, "data")
}

func TestDefault_CallerCancellationIsNotTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	server := fake.NewSender(fake.Hang())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := resilience.NewDefault(time.Second).Do(ctx, server)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
