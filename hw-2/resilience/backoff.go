package resilience

import (
	"context"
	"math"
	"time"
)

type Backoff interface {
	Delay(n int) time.Duration
}

var (
	_ Backoff = NoBackoff{}
	_ Backoff = Exponential{}
)

type NoBackoff struct{}

func (NoBackoff) Delay(int) time.Duration {
	return 0
}

type Exponential struct {
	Base   time.Duration
	Factor float64
	Max    time.Duration
}

func (e Exponential) Delay(n int) time.Duration {
	limit := e.Max
	if limit <= 0 {
		limit = math.MaxInt64
	}

	delay := float64(e.Base) * math.Pow(e.Factor, float64(n-1))
	if delay >= float64(limit) {
		return limit
	}

	return time.Duration(delay)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
