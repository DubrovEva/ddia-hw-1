package resilience

import (
	"context"
	"errors"
	"time"
)

var _ Client = (*Default)(nil)

type Default struct {
	timeout time.Duration
}

func NewDefault(timeout time.Duration) *Default {
	return &Default{timeout: timeout}
}

func (d *Default) Do(ctx context.Context, sender Sender) (Response, error) {
	attemptCtx, cancel := withLatencyBudget(ctx, d.timeout)
	defer cancel()

	resp, err := sender.Send(attemptCtx)
	if err != nil && ctx.Err() != nil {
		return Response{}, context.Cause(ctx)
	}

	var innerErr *OutcomeError
	if errors.As(err, &innerErr) {
		return Response{}, err
	}

	outcome := Classify(resp, err)
	if outcome == OutcomeSuccess {
		return resp, nil
	}

	return Response{}, &OutcomeError{Outcome: outcome, Response: resp, Err: err}
}
