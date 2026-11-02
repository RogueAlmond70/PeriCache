package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}

func TestPrometheusRecorderAndHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	p, err := NewPrometheusRecorder(reg)
	require.NoError(t, err)

	p.RecordOperation(OpCleanup, OutcomeSuccess)
	p.RecordGetResult(ResultHit)

	srv := httptest.NewServer(p.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	text := string(body)
	assert.Contains(t, text, "peri_peri_lru_cache_operations_total")
	assert.Contains(t, text, `operation="cleanup"`)
	assert.Contains(t, text, `outcome="success"`)
	assert.Contains(t, text, "peri_peri_lru_cache_get_results_total")
	assert.Contains(t, text, `result="hit"`)
}

func TestNewFromBackend(t *testing.T) {
	mem, err := NewFromBackend("memory", nil)
	require.NoError(t, err)
	_, ok := mem.(*MemoryRecorder)
	assert.True(t, ok)

	reg := prometheus.NewRegistry()
	both, err := NewFromBackend("both", reg)
	require.NoError(t, err)
	_, ok = both.(*MultiRecorder)
	assert.True(t, ok)

	_, err = NewFromBackend("nope", nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown metrics backend"))
}

func TestFanOutSnapshotAndHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	prom, err := NewPrometheusRecorder(reg)
	require.NoError(t, err)
	mem := NewMemoryRecorder()
	multi := FanOut(mem, prom)

	multi.RecordOperation(OpGet, OutcomeSuccess)
	multi.RecordGetResult(ResultMiss)

	snap := multi.Snapshot()
	assert.Equal(t, 1, snap.Get.Success)
	assert.Equal(t, 1, snap.Miss)

	require.NotNil(t, multi.Handler())
	require.NotNil(t, HTTPHandler(multi))
}
