package metrics

import "net/http"

// MultiRecorder fans events out to multiple Recorders.
type MultiRecorder struct {
	recorders []Recorder
}

// FanOut returns a Recorder that delegates to all provided recorders.
func FanOut(recorders ...Recorder) *MultiRecorder {
	out := make([]Recorder, 0, len(recorders))
	for _, r := range recorders {
		if r != nil {
			out = append(out, r)
		}
	}
	return &MultiRecorder{recorders: out}
}

func (m *MultiRecorder) RecordOperation(op Operation, outcome string) {
	for _, r := range m.recorders {
		r.RecordOperation(op, outcome)
	}
}

func (m *MultiRecorder) RecordGetResult(result string) {
	for _, r := range m.recorders {
		r.RecordGetResult(result)
	}
}

// Snapshot returns the first Snapshotter's snapshot, or a zero snapshot.
func (m *MultiRecorder) Snapshot() Snapshot {
	for _, r := range m.recorders {
		if s, ok := r.(Snapshotter); ok {
			return s.Snapshot()
		}
	}
	return Snapshot{}
}

// Handler returns the first HandlerProvider's handler, or nil.
func (m *MultiRecorder) Handler() http.Handler {
	for _, r := range m.recorders {
		if h, ok := r.(HandlerProvider); ok {
			if handler := h.Handler(); handler != nil {
				return handler
			}
		}
	}
	return nil
}
