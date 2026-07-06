package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/mayvqt/Augur/internal/config"
)

type healthServer struct {
	cfg     config.HealthConfig
	store   watchStore
	metrics *Metrics
	logger  *slog.Logger
	server  *http.Server
	ln      net.Listener
}

func newHealthServer(cfg config.HealthConfig, store watchStore, metrics *Metrics, logger *slog.Logger) *healthServer {
	mux := http.NewServeMux()
	server := &healthServer{cfg: cfg, store: store, metrics: metrics, logger: logger}
	mux.HandleFunc("/healthz", server.healthz)
	mux.HandleFunc("/readyz", server.readyz)
	mux.HandleFunc("/metrics", server.metricsz)
	server.server = &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
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

func (s *healthServer) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *healthServer) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *healthServer) metricsz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
