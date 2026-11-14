package resilience

import (
	"context"
	"time"
)

var _ MultiClient = (*Hedging)(nil)

type Hedging struct {
	delay   time.Duration
	total   time.Duration
	attempt *Default
}

func NewHedging(delay, total time.Duration) *Hedging {
	return &Hedging{
		delay:   delay,
		total:   total,
		attempt: NewDefault(total),
	}
}

type branchResult struct {
	resp Response
	err  error
}

func (h *Hedging) Do(ctx context.Context, senders []Sender) (Response, error) {
	if len(senders) == 0 {
		return Response{}, ErrNoReplicas
	}

	ctx, cancel := withLatencyBudget(ctx, h.total)
	defer cancel()

	results := make(chan branchResult, len(senders))
	launched := 0
	launch := func(senders []Sender) {
		for _, sender := range senders {
			go func() {
				resp, err := h.attempt.Do(ctx, sender)
				results <- branchResult{resp: resp, err: err}
			}()
		}
		launched += len(senders)
	}

	launch(senders[:1])

	timer := time.NewTimer(h.delay)
	defer timer.Stop()

	for finished := 0; ; {
		select {
		case <-timer.C:
			launch(senders[launched:])
		case result := <-results:
			if result.err == nil {
				return result.resp, nil
			}

			finished++
			if finished == len(senders) {
				return Response{}, result.err
			}

			launch(senders[launched:])
		case <-ctx.Done():
			if resp, ok := readySuccess(results); ok {
				return resp, nil
			}

			return Response{}, context.Cause(ctx)
		}
	}
}

func readySuccess(results <-chan branchResult) (Response, bool) {
	for {
		select {
		case result := <-results:
			if result.err == nil {
				return result.resp, true
			}
		default:
			return Response{}, false
		}
	}
}
