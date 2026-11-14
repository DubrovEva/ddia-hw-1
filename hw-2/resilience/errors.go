package resilience

import (
	"errors"
	"fmt"
)

var (
	ErrLatencyExhausted = errors.New("latency budget exhausted")
	ErrNoReplicas       = errors.New("no available replicas")
)

type OutcomeError struct {
	Outcome  Outcome
	Response Response
	Err      error
}

func (e *OutcomeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Outcome, e.Err)
	}

	return fmt.Sprintf("%s: status %d", e.Outcome, e.Response.StatusCode)
}

func (e *OutcomeError) Unwrap() error {
	return e.Err
}
