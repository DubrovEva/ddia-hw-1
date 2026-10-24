package loadgen

import "time"

type Ramp struct {
	Target   float64
	Duration time.Duration
}

func (r Ramp) RateAt(elapsed time.Duration) float64 {
	switch {
	case elapsed <= 0:
		return 0
	case r.Duration <= 0 || elapsed >= r.Duration:
		return r.Target
	default:
		return r.Target * elapsed.Seconds() / r.Duration.Seconds()
	}
}

func (r Ramp) DueAt(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	t := elapsed.Seconds()
	if r.Duration <= 0 {
		return r.Target * t
	}
	d := r.Duration.Seconds()
	if t < d {
		return r.Target * t * t / (2 * d)
	}

	return r.Target*d/2 + r.Target*(t-d)
}
