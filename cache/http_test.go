package cache

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"Nandos-Pair-Coding/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*httptest.Server, *ourCache) {
	t.Helper()
	c, _ := newTestCache(t, 10)
	mux := http.NewServeMux()
	RegisterRoutes(mux, c)
	return httptest.NewServer(mux), c
}

func TestHTTPHealthz(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPSetGetDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	profile := testProfile("cust-1")
	body, err := json.Marshal(profile)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/cache/cust-1?ttl=60", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/cache/cust-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "cust-1", got["customer_id"])

	req, err = http.NewRequest(http.MethodDelete, srv.URL+"/cache/cust-1", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/cache/cust-1")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTPValidationAndStats(t *testing.T) {
	srv, c := newTestServer(t)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut, srv.URL+"/cache/x?ttl=0", bytes.NewReader([]byte(`{"customer_id":"x"}`)))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	require.NoError(t, c.Set(t.Context(), "a", testProfile("a"), 60))

	resp, err = http.Get(srv.URL + "/cache/size")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/cache/keys")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(srv.URL + "/cache/stats")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	req, err = http.NewRequest(http.MethodDelete, srv.URL+"/cache", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestHTTPMetricsEndpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	rec, err := metrics.NewFromBackend("both", reg)
	require.NoError(t, err)

	logger := &mockLogger{}
	c, err := NewCache(t.Context(), logger, 10, rec, newMemoryStore())
	require.NoError(t, err)

	mux := http.NewServeMux()
	RegisterRoutes(mux, c)
	mux.Handle("GET /metrics", metrics.HTTPHandler(rec))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	require.NoError(t, c.Set(t.Context(), "a", testProfile("a"), 60))
	_, err = c.Get(t.Context(), "a")
	require.NoError(t, err)

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "peri_peri_lru_cache_operations_total")
}

func TestHTTPBatch(t *testing.T) {
	srv, _ := newTestServer(t)
	defer srv.Close()

	batch := []BatchRequest{
		{Key: "a", Data: testProfile("a"), TTL_Seconds: 60},
		{Key: "b", Data: testProfile("b"), TTL_Seconds: 60},
	}
	body, err := json.Marshal(batch)
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+"/cache/batch", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	getBody, err := json.Marshal([]BatchRequest{{Key: "a"}, {Key: "missing"}})
	require.NoError(t, err)
	resp, err = http.Post(srv.URL+"/cache/batch/get", "application/json", bytes.NewReader(getBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	delBody, err := json.Marshal([]BatchRequest{{Key: "a"}, {Key: "b"}})
	require.NoError(t, err)
	resp, err = http.Post(srv.URL+"/cache/batch/delete", "application/json", bytes.NewReader(delBody))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
