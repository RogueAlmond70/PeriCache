package cache

import (
	"Nandos-Pair-Coding/loyalty"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

const defaultHTTPTTLSeconds = 60

// RegisterRoutes mounts cache HTTP endpoints on mux.
//
//	PUT    /cache/{key}?ttl=60   body: LoyaltyProfile JSON
//	GET    /cache/{key}
//	DELETE /cache/{key}
//	DELETE /cache
//	GET    /cache/size
//	GET    /cache/keys
//	GET    /cache/stats
//	POST   /cache/batch          body: []BatchRequest
//	POST   /cache/batch/get      body: []BatchRequest (keys)
//	POST   /cache/batch/delete   body: []BatchRequest (keys)
//	GET    /healthz
func RegisterRoutes(mux *http.ServeMux, c Cache) {
	mux.HandleFunc("GET /healthz", handleHealthz)

	mux.HandleFunc("GET /cache/size", func(w http.ResponseWriter, r *http.Request) {
		handleSize(w, r, c)
	})
	mux.HandleFunc("GET /cache/keys", func(w http.ResponseWriter, r *http.Request) {
		handleKeys(w, r, c)
	})
	mux.HandleFunc("GET /cache/stats", func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r, c)
	})
	mux.HandleFunc("DELETE /cache", func(w http.ResponseWriter, r *http.Request) {
		handleClear(w, r, c)
	})

	mux.HandleFunc("PUT /cache/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleSet(w, r, c)
	})
	mux.HandleFunc("GET /cache/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleGet(w, r, c)
	})
	mux.HandleFunc("DELETE /cache/{key}", func(w http.ResponseWriter, r *http.Request) {
		handleDelete(w, r, c)
	})

	mux.HandleFunc("POST /cache/batch", func(w http.ResponseWriter, r *http.Request) {
		handleSetMany(w, r, c)
	})
	mux.HandleFunc("POST /cache/batch/get", func(w http.ResponseWriter, r *http.Request) {
		handleGetMany(w, r, c)
	})
	mux.HandleFunc("POST /cache/batch/delete", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteMany(w, r, c)
	})
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleSet(w http.ResponseWriter, r *http.Request, c Cache) {
	key := r.PathValue("key")
	ttl, err := parseTTL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var profile loyalty.LoyaltyProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := c.Set(r.Context(), key, profile, ttl); err != nil {
		writeCacheError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleGet(w http.ResponseWriter, r *http.Request, c Cache) {
	key := r.PathValue("key")
	profile, err := c.Get(r.Context(), key)
	if err != nil {
		writeCacheError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func handleDelete(w http.ResponseWriter, r *http.Request, c Cache) {
	key := r.PathValue("key")
	if err := c.Delete(r.Context(), key); err != nil {
		writeCacheError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleClear(w http.ResponseWriter, r *http.Request, c Cache) {
	if err := c.Clear(r.Context()); err != nil {
		writeCacheError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleSize(w http.ResponseWriter, r *http.Request, c Cache) {
	n, err := c.Size(r.Context())
	if err != nil {
		writeCacheError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"size": n})
}

func handleKeys(w http.ResponseWriter, r *http.Request, c Cache) {
	keys, err := c.Keys(r.Context())
	if err != nil {
		writeCacheError(w, err)
		return
	}
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"keys": keys})
}

func handleStats(w http.ResponseWriter, r *http.Request, c Cache) {
	writeJSON(w, http.StatusOK, c.Stats())
}

func handleSetMany(w http.ResponseWriter, r *http.Request, c Cache) {
	var batch []BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := c.SetMany(r.Context(), batch); err != nil {
		writeCacheError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleGetMany(w http.ResponseWriter, r *http.Request, c Cache) {
	var batch []BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profiles, err := c.GetMany(r.Context(), batch)
	if err != nil {
		writeCacheError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func handleDeleteMany(w http.ResponseWriter, r *http.Request, c Cache) {
	var batch []BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := c.DeleteMany(r.Context(), batch); err != nil {
		writeCacheError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseTTL(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("ttl")
	if raw == "" {
		return defaultHTTPTTLSeconds, nil
	}
	ttl, err := strconv.Atoi(raw)
	if err != nil {
		return 0, ErrInvalidTTL
	}
	if ttl < 1 {
		return 0, ErrInvalidTTL
	}
	return ttl, nil
}

func writeCacheError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrNoKey), errors.Is(err, ErrNoData), errors.Is(err, ErrInvalidTTL):
		writeError(w, http.StatusBadRequest, err)
	default:
		// Batch ops may join validation errors with others.
		if errors.Is(err, ErrNoKey) || errors.Is(err, ErrNoData) || errors.Is(err, ErrInvalidTTL) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
