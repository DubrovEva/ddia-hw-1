package loadgen

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRampRateAt(t *testing.T) {
	r := Ramp{Target: 100, Duration: 10 * time.Second}

	assert.InDelta(t, 0, r.RateAt(0), 1e-9)
	assert.InDelta(t, 50, r.RateAt(5*time.Second), 1e-9)
	assert.InDelta(t, 100, r.RateAt(10*time.Second), 1e-9)
	assert.InDelta(t, 100, r.RateAt(time.Minute), 1e-9)
	assert.InDelta(t, 100, Ramp{Target: 100}.RateAt(time.Second), 1e-9)
}

func TestRampDueAt(t *testing.T) {
	r := Ramp{Target: 100, Duration: 10 * time.Second}

	assert.InDelta(t, 0, r.DueAt(0), 1e-9)
	assert.InDelta(t, 125, r.DueAt(5*time.Second), 1e-9)
	assert.InDelta(t, 500, r.DueAt(10*time.Second), 1e-9)
	assert.InDelta(t, 1500, r.DueAt(20*time.Second), 1e-9)
	assert.InDelta(t, 200, Ramp{Target: 100}.DueAt(2*time.Second), 1e-9)
}

func TestScenarioRoundRobinAndCreateFirst(t *testing.T) {
	s := NewScenario([]string{"a", "b"}, 0, 1)

	first, second := s.Next(), s.Next()
	assert.Equal(t, "a", first.Target)
	assert.Equal(t, "b", second.Target)
	assert.Equal(t, OpCreate, first.Op)
	assert.Equal(t, OpCreate, second.Op)
}

func TestScenarioUsesKnownIDs(t *testing.T) {
	s := NewScenario([]string{"a"}, 0, 1)
	s.Created("a", 5)
	s.Created("a", 3)

	for range 500 {
		req := s.Next()
		if req.Op != OpGet && req.Op != OpUpdate {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(req.Path, "/items/"), 10, 64)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, id, int64(1))
		assert.LessOrEqual(t, id, int64(5))
	}
}

func TestScenarioMix(t *testing.T) {
	s := NewScenario([]string{"a"}, 0.1, 42)
	s.Created("a", 100)

	counts := map[Op]int{}
	const n = 10000
	for range n {
		counts[s.Next().Op]++
	}

	bad := counts[OpGetMissing] + counts[OpCreateInvalid]
	assert.InDelta(t, 0.1*n, bad, 0.02*n)
	assert.InDelta(t, 0.9*0.4*n, counts[OpGet], 0.03*n)
	assert.InDelta(t, 0.9*0.1*n, counts[OpList], 0.02*n)
	assert.InDelta(t, 0.9*0.25*n, counts[OpCreate], 0.03*n)
	assert.InDelta(t, 0.9*0.25*n, counts[OpUpdate], 0.03*n)
}

type fakeApp struct {
	mu     sync.Mutex
	nextID int64
	byOp   map[string]int
}

func (f *fakeApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.byOp[r.Method+" "+r.URL.Path]++
	switch {
	case r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		if !json.Valid(body) {
			w.WriteHeader(http.StatusBadRequest)

			return
		}
		f.nextID++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int64{"id": f.nextID})
	case strings.HasPrefix(r.URL.Path, "/items/"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/items/"), 10, 64)
		if id < 1 || id > f.nextID {
			w.WriteHeader(http.StatusNotFound)

			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func TestRunnerFollowsRamp(t *testing.T) {
	app := &fakeApp{byOp: map[string]int{}}
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)

	cfg := Config{
		Ramp:           Ramp{Target: 400, Duration: 200 * time.Millisecond},
		Duration:       500 * time.Millisecond,
		Workers:        16,
		ReportInterval: time.Hour,
		RequestTimeout: time.Second,
	}
	r, err := NewRunner(cfg, NewScenario([]string{srv.URL}, 0.2, 7), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	stats := r.Run(t.Context())

	want := cfg.Ramp.DueAt(cfg.Duration)
	assert.InDelta(t, want, float64(stats.Dispatched), want*0.15)
	assert.Equal(t, stats.Dispatched-stats.Dropped, stats.Completed())
	assert.Positive(t, stats.OK)
	assert.Positive(t, stats.ClientErr)
	assert.Zero(t, stats.ServerErr)
	assert.Zero(t, stats.Failed)
	assert.Positive(t, app.byOp["PUT /items/1"]+app.byOp["GET /items/1"])
}

func TestRunnerStopsOnCancel(t *testing.T) {
	srv := httptest.NewServer(&fakeApp{byOp: map[string]int{}})
	t.Cleanup(srv.Close)

	r, err := NewRunner(Config{
		Ramp:           Ramp{Target: 50},
		Workers:        4,
		ReportInterval: time.Hour,
		RequestTimeout: time.Second,
	}, NewScenario([]string{srv.URL}, 0, 1), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r.Run(ctx)

	assert.Less(t, time.Since(start), time.Second)
}

func TestNewRunnerValidation(t *testing.T) {
	s := NewScenario([]string{"a"}, 0, 1)
	logger := slog.New(slog.DiscardHandler)
	valid := Config{Ramp: Ramp{Target: 1}, Workers: 1, ReportInterval: time.Second}

	_, err := NewRunner(valid, s, logger)
	require.NoError(t, err)

	for name, mutate := range map[string]func(*Config){
		"target":  func(c *Config) { c.Ramp.Target = 0 },
		"workers": func(c *Config) { c.Workers = 0 },
		"report":  func(c *Config) { c.ReportInterval = 0 },
	} {
		cfg := valid
		mutate(&cfg)
		_, err := NewRunner(cfg, s, logger)
		assert.Error(t, err, name)
	}
}
