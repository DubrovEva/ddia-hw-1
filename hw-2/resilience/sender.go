package resilience

import (
	"context"
	"errors"
)

type Response struct {
	StatusCode int
	Body       []byte
}

type Sender interface {
	Send(ctx context.Context) (Response, error)
}

type Outcome int

const (
	OutcomeSuccess Outcome = iota
	OutcomeFastError
	OutcomeFailure
	OutcomeTimeout
)

func (o Outcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeFastError:
		return "fast error"
	case OutcomeFailure:
		return "failure"
	case OutcomeTimeout:
		return "timeout"
	default:
		return "unknown outcome"
	}
}

func Classify(resp Response, err error) Outcome {
	var outcomeErr *OutcomeError

	switch {
	case errors.As(err, &outcomeErr):
		return outcomeErr.Outcome
	case errors.Is(err, ErrLatencyExhausted), errors.Is(err, context.DeadlineExceeded):
		return OutcomeTimeout
	case err != nil:
		return OutcomeFailure
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return OutcomeSuccess
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return OutcomeFastError
	default:
		return OutcomeFailure
	}
}
