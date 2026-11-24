package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const maxBodyBytes = 1 << 20 
type Metrics struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewMetrics() *Metrics { return &Metrics{counts: make(map[string]int)} }

func (m *Metrics) inc(route string, status int) {
	m.mu.Lock()
	m.counts[fmt.Sprintf("%s|%d", route, status)]++
	m.mu.Unlock()
}

func (m *Metrics) Render() string {
	m.mu.Lock()
	keys := make([]string, 0, len(m.counts))
	for k := range m.counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# HELP http_requests_total Total HTTP requests by route and status.\n")
	b.WriteString("# TYPE http_requests_total counter\n")
	for _, k := range keys {
		parts := strings.SplitN(k, "|", 2)
		fmt.Fprintf(&b, "http_requests_total{route=%q,status=%q} %d\n", parts[0], parts[1], m.counts[k])
	}
	m.mu.Unlock()
	return b.String()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

type Server struct {
	store   *Store
	metrics *Metrics
}

func NewServer(store *Store) *Server {
	return &Server{store: store, metrics: NewMetrics()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /matches", s.instrument("POST /matches", s.createMatch))
	mux.HandleFunc("GET /players/{id}/stats", s.instrument("GET /players/{id}/stats", s.getStats))
	mux.HandleFunc("GET /leaderboard", s.instrument("GET /leaderboard", s.getLeaderboard))
	mux.HandleFunc("GET /healthz", s.instrument("GET /healthz", s.health))
	mux.HandleFunc("GET /metrics", s.metricsHandler)
	return mux
}

func (s *Server) instrument(route string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		s.metrics.inc(route, rec.status)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) createMatch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var m Match
	if err := dec.Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	switch err := s.store.Record(m); {
	case err == nil:
		writeJSON(w, http.StatusCreated, map[string]string{"id": m.ID})
	case errors.Is(err, ErrDuplicateMatch):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidMatch):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	st, ok := s.store.PlayerStats(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) getLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "limit must be a number from 1 to 100")
			return
		}
		limit = n
	}
	writeJSON(w, http.StatusOK, s.store.Leaderboard(limit))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(s.metrics.Render()))
}
