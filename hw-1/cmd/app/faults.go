package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"ddia/hw1/internal/env"
)

type faults struct {
	rate   float64
	jitter time.Duration
}

func faultsFromEnv() (faults, error) {
	rate, err := env.Float("FAULT_RATE", 0)
	if err != nil {
		return faults{}, err
	}
	if rate < 0 || rate > 1 {
		return faults{}, fmt.Errorf("env FAULT_RATE must be in [0, 1], got %g", rate)
	}
	jitter, err := env.Duration("LATENCY_JITTER", 0)
	if err != nil {
		return faults{}, err
	}
	if jitter < 0 {
		return faults{}, fmt.Errorf("env LATENCY_JITTER must not be negative, got %s", jitter)
	}

	return faults{rate: rate, jitter: jitter}, nil
}

func (f faults) wrap(h http.HandlerFunc) http.HandlerFunc {
	if f.rate == 0 && f.jitter == 0 {
		return h
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if f.jitter > 0 {
			timer := time.NewTimer(rand.N(f.jitter))
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				writeError(w, http.StatusServiceUnavailable, "request cancelled")

				return
			}
		}
		if f.rate > 0 && rand.Float64() < f.rate {
			writeError(w, http.StatusInternalServerError, "injected fault")

			return
		}
		h(w, r)
	}
}
