package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoryRecorder(t *testing.T) {
	m := NewMemoryRecorder()
	m.RecordOperation(OpSet, OutcomeSuccess)
	m.RecordOperation(OpSet, OutcomeFailure)
	m.RecordGetResult(ResultHit)
	m.RecordGetResult(ResultMiss)

	snap := m.Snapshot()
	assert.Equal(t, 1, snap.Set.Success)
	assert.Equal(t, 1, snap.Set.Failure)
	assert.Equal(t, 2, snap.Set.Attempted)
	assert.InDelta(t, 0.5, snap.Set.SuccessRatio, 0.001)
	assert.Equal(t, 1, snap.Hit)
	assert.Equal(t, 1, snap.Miss)
	assert.InDelta(t, 0.5, snap.HitRatio, 0.001)

	viaInterface, ok := SnapshotOf(m)
	assert.True(t, ok)
	assert.Equal(t, snap, viaInterface)

	var _ Recorder = m
}
