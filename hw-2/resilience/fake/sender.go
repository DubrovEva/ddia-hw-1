package fake

import (
	"context"
	"sync"
	"time"

	"ddia/hw2/resilience"
)

type Step func(ctx context.Context) (resilience.Response, error)

func OK(body string) Step {
	return func(context.Context) (resilience.Response, error) {
		return resilience.Response{StatusCode: 200, Body: []byte(body)}, nil
	}
}

func Status(code int) Step {
	return func(context.Context) (resilience.Response, error) {
		return resilience.Response{StatusCode: code}, nil
	}
}

func Hang() Step {
	return func(ctx context.Context) (resilience.Response, error) {
		<-ctx.Done()

		return resilience.Response{}, ctx.Err()
	}
}

func After(d time.Duration, step Step) Step {
	return func(ctx context.Context) (resilience.Response, error) {
		timer := time.NewTimer(d)
		defer timer.Stop()

		select {
		case <-timer.C:
			return step(ctx)
		case <-ctx.Done():
			return resilience.Response{}, ctx.Err()
		}
	}
}

type Sender struct {
	mu    sync.Mutex
	steps []Step
	calls int
}

func NewSender(steps ...Step) *Sender {
	return &Sender{steps: steps}
}

func (s *Sender) Send(ctx context.Context) (resilience.Response, error) {
	s.mu.Lock()
	step := s.steps[min(s.calls, len(s.steps)-1)]
	s.calls++
	s.mu.Unlock()

	return step(ctx)
}

func (s *Sender) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.calls
}
