package loadgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const tickInterval = 10 * time.Millisecond

type Config struct {
	Ramp           Ramp
	Duration       time.Duration
	Workers        int
	ReportInterval time.Duration
	RequestTimeout time.Duration
}

type Stats struct {
	Dispatched int64
	Dropped    int64
	OK         int64
	ClientErr  int64
	ServerErr  int64
	Failed     int64
}

func (s Stats) Completed() int64 {
	return s.OK + s.ClientErr + s.ServerErr + s.Failed
}

type counters struct {
	dispatched, dropped, ok, clientErr, serverErr, failed atomic.Int64
}

func (c *counters) snapshot() Stats {
	return Stats{
		Dispatched: c.dispatched.Load(),
		Dropped:    c.dropped.Load(),
		OK:         c.ok.Load(),
		ClientErr:  c.clientErr.Load(),
		ServerErr:  c.serverErr.Load(),
		Failed:     c.failed.Load(),
	}
}

type Runner struct {
	cfg      Config
	scenario *Scenario
	client   *http.Client
	logger   *slog.Logger
	stats    counters
}

func NewRunner(cfg Config, scenario *Scenario, logger *slog.Logger) (*Runner, error) {
	if cfg.Ramp.Target <= 0 {
		return nil, errors.New("loadgen: target rps must be positive")
	}
	if cfg.Workers <= 0 {
		return nil, errors.New("loadgen: workers must be positive")
	}
	if cfg.ReportInterval <= 0 {
		return nil, errors.New("loadgen: report interval must be positive")
	}

	return &Runner{
		cfg:      cfg,
		scenario: scenario,
		client:   &http.Client{Timeout: cfg.RequestTimeout},
		logger:   logger,
	}, nil
}

func (r *Runner) Run(ctx context.Context) Stats {
	jobs := make(chan Request, r.cfg.Workers)
	var wg sync.WaitGroup
	for range r.cfg.Workers {
		wg.Go(func() {
			for req := range jobs {
				r.do(ctx, req)
			}
		})
	}
	defer func() {
		close(jobs)
		wg.Wait()
	}()

	start := time.Now()
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()
	report := time.NewTicker(r.cfg.ReportInterval)
	defer report.Stop()
	last := r.stats.snapshot()

	for {
		select {
		case <-ctx.Done():
			return r.stats.snapshot()
		case <-report.C:
			last = r.report(time.Since(start), last)
		case now := <-tick.C:
			elapsed := now.Sub(start)
			if r.cfg.Duration > 0 && elapsed >= r.cfg.Duration {
				return r.stats.snapshot()
			}
			r.dispatch(jobs, int64(r.cfg.Ramp.DueAt(elapsed)))
		}
	}
}

func (r *Runner) dispatch(jobs chan<- Request, due int64) {
	for r.stats.dispatched.Load() < due {
		r.stats.dispatched.Add(1)
		select {
		case jobs <- r.scenario.Next():
		default:
			r.stats.dropped.Add(1)
		}
	}
}

func (r *Runner) do(ctx context.Context, req Request) {
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.Target+req.Path, body)
	if err != nil {
		r.stats.failed.Add(1)

		return
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(httpReq)
	if err != nil {
		if ctx.Err() == nil {
			r.stats.failed.Add(1)
			r.logger.Debug("request failed", "target", req.Target, "op", req.Op, "err", err)
		}

		return
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		r.stats.serverErr.Add(1)
	case resp.StatusCode >= http.StatusBadRequest:
		r.stats.clientErr.Add(1)
	default:
		r.stats.ok.Add(1)
	}

	if req.Op == OpCreate && resp.StatusCode == http.StatusCreated {
		var created struct {
			ID int64 `json:"id"`
		}
		if json.NewDecoder(resp.Body).Decode(&created) == nil {
			r.scenario.Created(req.Target, created.ID)
		}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func (r *Runner) report(elapsed time.Duration, prev Stats) Stats {
	cur := r.stats.snapshot()
	interval := r.cfg.ReportInterval.Seconds()
	phase := "steady"
	if elapsed < r.cfg.Ramp.Duration {
		phase = "ramp-up"
	}
	r.logger.Info("load",
		"phase", phase,
		"elapsed", elapsed.Round(time.Second).String(),
		"target_rps", fmt.Sprintf("%.1f", r.cfg.Ramp.RateAt(elapsed)),
		"actual_rps", fmt.Sprintf("%.1f", float64(cur.Completed()-prev.Completed())/interval),
		"ok", cur.OK-prev.OK,
		"4xx", cur.ClientErr-prev.ClientErr,
		"5xx", cur.ServerErr-prev.ServerErr,
		"failed", cur.Failed-prev.Failed,
		"dropped", cur.Dropped-prev.Dropped,
	)

	return cur
}
