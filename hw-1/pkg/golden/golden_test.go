package golden

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfig = Config{Pod: "pod-1", Datacenter: "dc1", Deployment: "items"}

func newTestInstrumenter(t *testing.T) (*Instrumenter, *prometheus.Registry) {
	t.Helper()

	reg := prometheus.NewRegistry()
	ins, err := New(testConfig, reg)
	require.NoError(t, err)

	return ins, reg
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))

	return rec
}

func TestNewInvalidConfig(t *testing.T) {
	_, err := New(Config{}, prometheus.NewRegistry())
	assert.Error(t, err)
}

func TestNewDuplicateRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	_, err := New(testConfig, reg)
	require.NoError(t, err)

	_, err = New(testConfig, reg)
	assert.Error(t, err)
}

func TestHandleRecordsSignals(t *testing.T) {
	ins, reg := newTestInstrumenter(t)
	mux := http.NewServeMux()
	ins.Handle(mux, "GET /ok/{id}", "ok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	})
	ins.Handle(mux, "GET /missing", "missing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	ins.Handle(mux, "POST /fail", "fail", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rec := serve(mux, "GET", "/ok/42")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "42", rec.Body.String())
	serve(mux, "GET", "/ok/43")
	serve(mux, "GET", "/missing")
	serve(mux, "POST", "/fail")

	expected := `
# HELP golden_requests_total Handled requests per handler and status code.
# TYPE golden_requests_total counter
golden_requests_total{code="200",datacenter="dc1",deployment="items",handler="ok",method="GET",pod="pod-1",signal="traffic"} 2
golden_requests_total{code="404",datacenter="dc1",deployment="items",handler="missing",method="GET",pod="pod-1",signal="traffic"} 1
golden_requests_total{code="500",datacenter="dc1",deployment="items",handler="fail",method="POST",pod="pod-1",signal="traffic"} 1
# HELP golden_errors_total Requests finished with HTTP 4xx/5xx per handler and status code.
# TYPE golden_errors_total counter
golden_errors_total{code="404",datacenter="dc1",deployment="items",error_class="4xx",handler="missing",method="GET",pod="pod-1",signal="errors"} 1
golden_errors_total{code="500",datacenter="dc1",deployment="items",error_class="5xx",handler="fail",method="POST",pod="pod-1",signal="errors"} 1
`
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(expected), MetricRequests, MetricErrors))

	assert.Equal(t, map[string]uint64{"ok": 2, "missing": 1, "fail": 1}, latencyCounts(t, reg))
}

func TestWrapKeepsFirstStatus(t *testing.T) {
	ins, reg := newTestInstrumenter(t)
	h := ins.Wrap("created", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusInternalServerError)
	}))

	serve(h, "PUT", "/")

	assert.Equal(t, 1.0, testutil.ToFloat64(ins.requests.WithLabelValues("created", "PUT", "201")))
	assert.Equal(t, 0, testutil.CollectAndCount(ins.errors))
	assert.Equal(t, map[string]uint64{"created": 1}, latencyCounts(t, reg))
}

func TestWrapLatencyLabels(t *testing.T) {
	ins, reg := newTestInstrumenter(t)
	serve(ins.Wrap("h", http.NotFoundHandler()), "GET", "/")

	families, err := reg.Gather()
	require.NoError(t, err)

	var labels map[string]string
	for _, mf := range families {
		if mf.GetName() == MetricLatency {
			labels = map[string]string{}
			for _, lp := range mf.GetMetric()[0].GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
		}
	}
	assert.Equal(t, map[string]string{
		LabelPod:        "pod-1",
		LabelDatacenter: "dc1",
		LabelDeployment: "items",
		LabelSignal:     SignalLatency,
		LabelHandler:    "h",
		LabelMethod:     "GET",
	}, labels)
}

func latencyCounts(t *testing.T, reg *prometheus.Registry) map[string]uint64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	counts := map[string]uint64{}
	for _, mf := range families {
		if mf.GetName() != MetricLatency {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == LabelHandler {
					counts[lp.GetValue()] += m.GetHistogram().GetSampleCount()
				}
			}
		}
	}

	return counts
}
