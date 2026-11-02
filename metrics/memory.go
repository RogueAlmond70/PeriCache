package metrics

import "sync"

// MemoryRecorder keeps in-process counters and can produce a Snapshot.
type MemoryRecorder struct {
	mu sync.Mutex

	hit  int
	miss int

	ops map[Operation]*opStats
}

type opStats struct {
	attempted    int
	success      int
	failure      int
	successRatio float64
	failureRatio float64
}

// NewMemoryRecorder returns a Recorder suitable for tests and /cache/stats.
func NewMemoryRecorder() *MemoryRecorder {
	return &MemoryRecorder{
		ops: map[Operation]*opStats{
			OpGet:     {},
			OpSet:     {},
			OpDelete:  {},
			OpClear:   {},
			OpSize:    {},
			OpKeys:    {},
			OpCleanup: {},
		},
	}
}

func (m *MemoryRecorder) RecordOperation(op Operation, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, ok := m.ops[op]
	if !ok {
		stats = &opStats{}
		m.ops[op] = stats
	}
	stats.attempted++
	switch outcome {
	case OutcomeSuccess:
		stats.success++
	case OutcomeFailure:
		stats.failure++
	}
	stats.recalculate()
}

func (m *MemoryRecorder) RecordGetResult(result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch result {
	case ResultHit:
		m.hit++
	case ResultMiss:
		m.miss++
	}
}

func (o *opStats) recalculate() {
	total := o.success + o.failure
	if total == 0 {
		o.successRatio = 0
		o.failureRatio = 0
		return
	}
	o.successRatio = float64(o.success) / float64(total)
	o.failureRatio = float64(o.failure) / float64(total)
}

// Snapshot returns a concurrency-safe copy of counters and ratios.
func (m *MemoryRecorder) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	hitRatio, missRatio := 0.0, 0.0
	total := m.hit + m.miss
	if total > 0 {
		hitRatio = float64(m.hit) / float64(total)
		missRatio = float64(m.miss) / float64(total)
	}

	return Snapshot{
		Hit:       m.hit,
		Miss:      m.miss,
		HitRatio:  hitRatio,
		MissRatio: missRatio,
		Get:       toOperationStats(m.ops[OpGet]),
		Set:       toOperationStats(m.ops[OpSet]),
		Delete:    toOperationStats(m.ops[OpDelete]),
		Clear:     toOperationStats(m.ops[OpClear]),
		Size:      toOperationStats(m.ops[OpSize]),
		Keys:      toOperationStats(m.ops[OpKeys]),
		Cleanup:   toOperationStats(m.ops[OpCleanup]),
	}
}

func toOperationStats(o *opStats) OperationStats {
	if o == nil {
		return OperationStats{}
	}
	return OperationStats{
		Attempted:    o.attempted,
		Success:      o.success,
		Failure:      o.failure,
		SuccessRatio: o.successRatio,
		FailureRatio: o.failureRatio,
	}
}
