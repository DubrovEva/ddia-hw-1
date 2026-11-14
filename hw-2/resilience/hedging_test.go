package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ddia/hw2/resilience"
	"ddia/hw2/resilience/fake"
)

func TestHedging_FastPrimaryIsNotHedged(t *testing.T) {
	a := fake.NewSender(fake.OK("from a"))
	b := fake.NewSender(fake.OK("from b"))

	resp, err := resilience.NewHedging(50*time.Millisecond, time.Second).Do(t.Context(), []resilience.Sender{a, b})

	requireBody(t, resp, err, "from a")
	requireCalls(t, b, 0)
}

func TestHedging_SlowPrimaryIsHedgedAfterDelay(t *testing.T) {
	const delay = 50 * time.Millisecond

	a := fake.NewSender(fake.Hang())
	b := fake.NewSender(fake.After(10*time.Millisecond, fake.OK("from b")))
	c := fake.NewSender(fake.Hang())

	start := time.Now()
	resp, err := resilience.NewHedging(delay, time.Second).Do(t.Context(), []resilience.Sender{a, b, c})
	elapsed := time.Since(start)

	requireBody(t, resp, err, "from b")
	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
	requireCalls(t, c, 1)

	if elapsed < delay || elapsed > 10*delay {
		t.Fatalf("elapsed = %v, want slightly above %v", elapsed, delay)
	}
}

func TestHedging_PrimaryErrorHedgesImmediately(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.OK("from b"))

	start := time.Now()
	resp, err := resilience.NewHedging(time.Second, 2*time.Second).Do(t.Context(), []resilience.Sender{a, b})
	elapsed := time.Since(start)

	requireBody(t, resp, err, "from b")

	if limit := 500 * time.Millisecond; elapsed > limit {
		t.Fatalf("elapsed = %v, want under %v", elapsed, limit)
	}
}

func TestHedging_FailsEarlyWhenAllReplicasFail(t *testing.T) {
	a := fake.NewSender(fake.Status(503))
	b := fake.NewSender(fake.Status(404))

	start := time.Now()
	_, err := resilience.NewHedging(10*time.Millisecond, 2*time.Second).Do(t.Context(), []resilience.Sender{a, b})
	elapsed := time.Since(start)

	var outcomeErr *resilience.OutcomeError
	if !errors.As(err, &outcomeErr) {
		t.Fatalf("err = %v, want OutcomeError", err)
	}

	if limit := 500 * time.Millisecond; elapsed > limit {
		t.Fatalf("elapsed = %v, want under %v", elapsed, limit)
	}
}

func TestHedging_StopsWhenLatencyBudgetIsExhausted(t *testing.T) {
	a := fake.NewSender(fake.Hang())
	b := fake.NewSender(fake.Hang())

	_, err := resilience.NewHedging(10*time.Millisecond, 50*time.Millisecond).Do(t.Context(), []resilience.Sender{a, b})

	if !errors.Is(err, resilience.ErrLatencyExhausted) {
		t.Fatalf("err = %v, want ErrLatencyExhausted", err)
	}

	requireCalls(t, a, 1)
	requireCalls(t, b, 1)
}

type cancelProbe struct {
	cancelled chan struct{}
}

func (p cancelProbe) Send(ctx context.Context) (resilience.Response, error) {
	<-ctx.Done()
	close(p.cancelled)

	return resilience.Response{}, ctx.Err()
}

func TestHedging_CancelsLosersAfterSuccess(t *testing.T) {
	loser := cancelProbe{cancelled: make(chan struct{})}
	winner := fake.NewSender(fake.OK("winner"))

	resp, err := resilience.NewHedging(10*time.Millisecond, time.Minute).Do(t.Context(), []resilience.Sender{loser, winner})

	requireBody(t, resp, err, "winner")

	select {
	case <-loser.cancelled:
	case <-time.After(time.Second):
		t.Fatal("losing subrequest was not cancelled")
	}
}
