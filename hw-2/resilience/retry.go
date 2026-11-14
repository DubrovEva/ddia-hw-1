package resilience

import (
	"context"
	"errors"
	"time"
)

var (
	_ Client      = (*Retry)(nil)
	_ MultiClient = (*MultiRetry)(nil)
)

type RetryConfig struct {
	Budget  RetryBudget
	Latency LatencyBudget
	Backoff Backoff
}

type MultiRetry struct {
	cfg      RetryConfig
	attempt  *Default
	balancer Balancer
}

func NewMultiRetry(cfg RetryConfig, balancer Balancer) *MultiRetry {
	if cfg.Backoff == nil {
		cfg.Backoff = NoBackoff{}
	}

	if balancer == nil {
		balancer = RoundRobin{}
	}

	return &MultiRetry{
		cfg:      cfg,
		attempt:  NewDefault(cfg.Latency.Subrequest),
		balancer: balancer,
	}
}

func (r *MultiRetry) Do(ctx context.Context, senders []Sender) (Response, error) {
	ctx, cancel := withLatencyBudget(ctx, r.cfg.Latency.Total)
	defer cancel()

	var (
		budget      = r.cfg.Budget
		heavyErrors int
		from        int
		delay       time.Duration
		lastErr     error
	)

	for {
		replica, err := r.balancer.Pick(from, len(senders))
		if err != nil {
			return Response{}, errors.Join(err, lastErr)
		}

		if err := sleep(ctx, delay); err != nil {
			return Response{}, err
		}

		resp, err := r.attempt.Do(ctx, senders[replica])
		if err == nil {
			r.balancer.Report(replica, OutcomeSuccess)

			return resp, nil
		}

		var outcomeErr *OutcomeError
		if !errors.As(err, &outcomeErr) {
			return Response{}, err
		}

		r.balancer.Report(replica, outcomeErr.Outcome)

		if budget.spend(outcomeErr.Outcome) {
			return Response{}, err
		}

		lastErr = err
		from = replica + 1
		delay = 0

		if outcomeErr.Outcome != OutcomeFastError {
			heavyErrors++
			delay = r.cfg.Backoff.Delay(heavyErrors)
		}
	}
}

type Retry struct {
	multi *MultiRetry
}

func NewRetry(cfg RetryConfig) *Retry {
	return &Retry{multi: NewMultiRetry(cfg, RoundRobin{})}
}

func (r *Retry) Do(ctx context.Context, sender Sender) (Response, error) {
	return r.multi.Do(ctx, []Sender{sender})
}
