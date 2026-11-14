package resilience

import (
	"context"
	"time"
)

type RetryBudget struct {
	FastErrors int
	Failures   int
	Timeouts   int
}

type LatencyBudget struct {
	Total      time.Duration
	Subrequest time.Duration
}

func withLatencyBudget(ctx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeoutCause(ctx, budget, ErrLatencyExhausted)
}

func (b *RetryBudget) spend(outcome Outcome) (exhausted bool) {
	switch outcome {
	case OutcomeFastError:
		b.FastErrors--

		return b.FastErrors <= 0
	case OutcomeFailure:
		b.Failures--

		return b.Failures <= 0
	case OutcomeTimeout:
		b.Timeouts--

		return b.Timeouts <= 0
	default:
		return false
	}
}
