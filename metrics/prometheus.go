package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "peri_peri"
	subsystem = "lru_cache"
)

// PrometheusRecorder records cache metrics using Prometheus CounterVecs.
type PrometheusRecorder struct {
	operations *prometheus.CounterVec
	getResults *prometheus.CounterVec
	registry   prometheus.Gatherer
}

// NewPrometheusRecorder registers metrics on the given Registerer (typically
// prometheus.DefaultRegisterer). Pass nil to use the default registerer/gatherer.
func NewPrometheusRecorder(reg prometheus.Registerer) (*PrometheusRecorder, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	operations := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "operations_total",
			Help:      "Total cache operations by type and outcome.",
		},
		[]string{"operation", "outcome"},
	)
	getResults := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "get_results_total",
			Help:      "Total cache get hit/miss results.",
		},
		[]string{"result"},
	)

	if err := reg.Register(operations); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			operations = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			return nil, err
		}
	}
	if err := reg.Register(getResults); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			getResults = are.ExistingCollector.(*prometheus.CounterVec)
		} else {
			return nil, err
		}
	}

	gatherer, ok := reg.(prometheus.Gatherer)
	if !ok {
		gatherer = prometheus.DefaultGatherer
	}

	return &PrometheusRecorder{
		operations: operations,
		getResults: getResults,
		registry:   gatherer,
	}, nil
}

func (p *PrometheusRecorder) RecordOperation(op Operation, outcome string) {
	p.operations.WithLabelValues(string(op), outcome).Inc()
}

func (p *PrometheusRecorder) RecordGetResult(result string) {
	p.getResults.WithLabelValues(result).Inc()
}

// Handler exposes Prometheus metrics in text exposition format.
func (p *PrometheusRecorder) Handler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
}
