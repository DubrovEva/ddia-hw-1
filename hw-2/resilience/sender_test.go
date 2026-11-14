package resilience_test

import (
	"context"
	"errors"
	"testing"

	"ddia/hw2/resilience"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		resp resilience.Response
		err  error
		want resilience.Outcome
	}{
		{"2xx", resilience.Response{StatusCode: 204}, nil, resilience.OutcomeSuccess},
		{"4xx", resilience.Response{StatusCode: 404}, nil, resilience.OutcomeFastError},
		{"5xx", resilience.Response{StatusCode: 503}, nil, resilience.OutcomeFailure},
		{"deadline", resilience.Response{}, context.DeadlineExceeded, resilience.OutcomeTimeout},
		{"transport error", resilience.Response{}, errors.New("connection refused"), resilience.OutcomeFailure},
		{"latency exhausted", resilience.Response{}, resilience.ErrLatencyExhausted, resilience.OutcomeTimeout},
		{
			"outcome error",
			resilience.Response{},
			&resilience.OutcomeError{Outcome: resilience.OutcomeFastError},
			resilience.OutcomeFastError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resilience.Classify(tt.resp, tt.err); got != tt.want {
				t.Errorf("Classify() = %s, want %s", got, tt.want)
			}
		})
	}
}
