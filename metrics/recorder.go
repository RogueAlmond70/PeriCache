package metrics

import "net/http"

// Operation identifies a cache API operation for metrics.
type Operation string

const (
	OpGet     Operation = "get"
	OpSet     Operation = "set"
	OpDelete  Operation = "delete"
	OpClear   Operation = "clear"
	OpSize    Operation = "size"
	OpKeys    Operation = "keys"
	OpCleanup Operation = "cleanup"
)

// Outcome labels for RecordOperation.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

// GetResult labels for RecordGetResult.
const (
	ResultHit  = "hit"
	ResultMiss = "miss"
)

// Recorder records cache domain metric events.
// Implementations must be safe for concurrent use.
type Recorder interface {
	RecordOperation(op Operation, outcome string)
	RecordGetResult(result string)
}

// Snapshotter is optionally implemented by recorders that can serve JSON stats.
type Snapshotter interface {
	Snapshot() Snapshot
}

// Snapshot is a point-in-time view of in-process counters (memory backend).
type Snapshot struct {
	Hit       int            `json:"hit"`
	Miss      int            `json:"miss"`
	HitRatio  float64        `json:"hit_ratio"`
	MissRatio float64        `json:"miss_ratio"`
	Get       OperationStats `json:"get"`
	Set       OperationStats `json:"set"`
	Delete    OperationStats `json:"delete"`
	Clear     OperationStats `json:"clear"`
	Size      OperationStats `json:"size"`
	Keys      OperationStats `json:"keys"`
	Cleanup   OperationStats `json:"cleanup"`
}

// OperationStats holds per-operation counters and derived ratios.
type OperationStats struct {
	Attempted    int     `json:"attempted"`
	Success      int     `json:"success"`
	Failure      int     `json:"failure"`
	SuccessRatio float64 `json:"success_ratio"`
	FailureRatio float64 `json:"failure_ratio"`
}

// Handler returns an HTTP handler for Prometheus exposition, or nil if unsupported.
type HandlerProvider interface {
	Handler() http.Handler
}
