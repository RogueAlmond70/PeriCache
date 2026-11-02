package metrics

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Backend names for NewFromBackend.
const (
	BackendMemory     = "memory"
	BackendPrometheus = "prometheus"
	BackendBoth       = "both"
)

// NewFromBackend builds a Recorder from a backend name.
// Supported: memory | prometheus | both.
func NewFromBackend(backend string, reg prometheus.Registerer) (Recorder, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", BackendMemory:
		return NewMemoryRecorder(), nil
	case BackendPrometheus:
		return NewPrometheusRecorder(reg)
	case BackendBoth:
		mem := NewMemoryRecorder()
		prom, err := NewPrometheusRecorder(reg)
		if err != nil {
			return nil, err
		}
		return FanOut(mem, prom), nil
	default:
		return nil, fmt.Errorf("unknown metrics backend %q (want memory|prometheus|both)", backend)
	}
}

// HTTPHandler returns a Prometheus scrape handler if the recorder supports it.
func HTTPHandler(r Recorder) http.Handler {
	if h, ok := r.(HandlerProvider); ok {
		return h.Handler()
	}
	return nil
}

// SnapshotOf returns a snapshot when available.
func SnapshotOf(r Recorder) (Snapshot, bool) {
	if s, ok := r.(Snapshotter); ok {
		return s.Snapshot(), true
	}
	return Snapshot{}, false
}
