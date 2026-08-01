package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/mayvqt/Augur/internal/config"
)

type healthServer struct {
	cfg     config.HealthConfig
	store   subscriptionStore
	metrics *Metrics
	logger  *slog.Logger
	server  *http.Server
	ln      net.Listener
	ready   atomic.Bool
}

func newHealthServer(cfg config.HealthConfig, store subscriptionStore, metrics *Metrics, logger *slog.Logger) *healthServer {
	mux := http.NewServeMux()
	server := &healthServer{cfg: cfg, store: store, metrics: metrics, logger: logger}
	mux.HandleFunc("/healthz", server.healthz)
	mux.HandleFunc("/readyz", server.readyz)
	mux.HandleFunc("/metrics", server.metricsz)
	server.server = &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return server
}

func (s *healthServer) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Address)
	if err != nil {
		return err
	}
	s.ln = ln
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("health server stopped", "error", err)
		}
	}()
	s.logger.Info("health server started", "address", ln.Addr().String())
	return nil
}

func (s *healthServer) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.server.Shutdown(shutdownCtx)
}

func (s *healthServer) SetReady(ready bool) {
	if s != nil {
		s.ready.Store(ready)
	}
}

func (s *healthServer) healthz(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *healthServer) readyz(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if !s.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *healthServer) metricsz(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
	return false
}
