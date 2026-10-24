package golden

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	MetricLatency  = "golden_request_duration_seconds"
	MetricRequests = "golden_requests_total"
	MetricErrors   = "golden_errors_total"

	LabelPod        = "pod"
	LabelDatacenter = "datacenter"
	LabelDeployment = "deployment"
	LabelSignal     = "signal"
	LabelHandler    = "handler"
	LabelMethod     = "method"
	LabelCode       = "code"
	LabelErrorClass = "error_class"

	SignalLatency = "latency"
	SignalTraffic = "traffic"
	SignalErrors  = "errors"
)

var DefaultBuckets = []float64{
	0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.075,
	0.1, 0.15, 0.2, 0.3, 0.5, 0.75, 1, 2.5, 5,
}

type Instrumenter struct {
	latency  *prometheus.HistogramVec
	requests *prometheus.CounterVec
	errors   *prometheus.CounterVec
}

func New(cfg Config, reg prometheus.Registerer) (*Instrumenter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	base := prometheus.Labels{
		LabelPod:        cfg.Pod,
		LabelDatacenter: cfg.Datacenter,
		LabelDeployment: cfg.Deployment,
	}

	i := &Instrumenter{
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        MetricLatency,
			Help:        "Request latency per handler.",
			Buckets:     DefaultBuckets,
			ConstLabels: withSignal(base, SignalLatency),
		}, []string{LabelHandler, LabelMethod}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        MetricRequests,
			Help:        "Handled requests per handler and status code.",
			ConstLabels: withSignal(base, SignalTraffic),
		}, []string{LabelHandler, LabelMethod, LabelCode}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        MetricErrors,
			Help:        "Requests finished with HTTP 4xx/5xx per handler and status code.",
			ConstLabels: withSignal(base, SignalErrors),
		}, []string{LabelHandler, LabelMethod, LabelCode, LabelErrorClass}),
	}

	for _, c := range []prometheus.Collector{i.latency, i.requests, i.errors} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return i, nil
}

func (i *Instrumenter) Handle(mux *http.ServeMux, pattern, handlerID string, h http.HandlerFunc) {
	mux.Handle(pattern, i.Wrap(handlerID, h))
}

func (i *Instrumenter) Wrap(handlerID string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}

		h.ServeHTTP(sw, r)

		i.observe(handlerID, r.Method, sw.code, time.Since(start))
	})
}

func (i *Instrumenter) observe(handlerID, method string, code int, d time.Duration) {
	status := strconv.Itoa(code)

	i.latency.WithLabelValues(handlerID, method).Observe(d.Seconds())
	i.requests.WithLabelValues(handlerID, method, status).Inc()
	if class := errorClass(code); class != "" {
		i.errors.WithLabelValues(handlerID, method, status, class).Inc()
	}
}

func errorClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	default:
		return ""
	}
}

func withSignal(base prometheus.Labels, signal string) prometheus.Labels {
	out := make(prometheus.Labels, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out[LabelSignal] = signal

	return out
}

type statusWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.code = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true

	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
