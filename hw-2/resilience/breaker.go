package resilience

import (
	"sync"
	"time"
)

var _ Balancer = (*CircuitBreaker)(nil)

type BreakerConfig struct {
	Window    int
	Threshold float64
	Cooldown  time.Duration
}

type CircuitBreaker struct {
	cfg      BreakerConfig
	mu       sync.Mutex
	replicas map[int]*replicaStats
}

func NewCircuitBreaker(cfg BreakerConfig) *CircuitBreaker {
	if cfg.Window < 1 {
		panic("breaker window must be positive")
	}

	return &CircuitBreaker{
		cfg:      cfg,
		replicas: make(map[int]*replicaStats),
	}
}

func (b *CircuitBreaker) Pick(from, replicas int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	for i := range replicas {
		replica := (from + i) % replicas
		if !b.stats(replica).isOpen(now) {
			return replica, nil
		}
	}

	return 0, ErrNoReplicas
}

func (b *CircuitBreaker) Report(replica int, outcome Outcome) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	stats := b.stats(replica)
	if stats.isOpen(now) {
		return
	}

	stats.record(outcome != OutcomeSuccess)

	if stats.filled == len(stats.window) &&
		float64(stats.failures)/float64(len(stats.window)) > b.cfg.Threshold {
		stats.open(now.Add(b.cfg.Cooldown))
	}
}

func (b *CircuitBreaker) stats(replica int) *replicaStats {
	stats, ok := b.replicas[replica]
	if !ok {
		stats = &replicaStats{window: make([]bool, b.cfg.Window)}
		b.replicas[replica] = stats
	}

	return stats
}

type replicaStats struct {
	window    []bool
	next      int
	filled    int
	failures  int
	openUntil time.Time
}

func (s *replicaStats) isOpen(now time.Time) bool {
	return now.Before(s.openUntil)
}

func (s *replicaStats) record(failed bool) {
	if s.filled == len(s.window) {
		if s.window[s.next] {
			s.failures--
		}
	} else {
		s.filled++
	}

	s.window[s.next] = failed
	if failed {
		s.failures++
	}

	s.next = (s.next + 1) % len(s.window)
}

func (s *replicaStats) open(until time.Time) {
	clear(s.window)
	s.next, s.filled, s.failures = 0, 0, 0
	s.openUntil = until
}
