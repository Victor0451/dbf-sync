package metrics

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Server manages the metrics HTTP endpoint lifecycle.
type Server struct {
	collector  Collector
	httpServer *http.Server
	cancel     context.CancelFunc
	sampler    *resourceSampler
}

// NopServer is a no-op server used when metrics is disabled.
var NopServer = &nopServer{}

type nopServer struct{}

func (n *nopServer) Start() error { return nil }
func (n *nopServer) Shutdown()    {}

// NewServer creates a metrics server. If enabled is false, returns a nop server.
// The collector is used for metrics collection. The db is used for resource sampling.
func NewServer(port int, enabled bool, collector Collector, db *sql.DB) *Server {
	if !enabled {
		return &Server{} // Returns empty server (acts as nop)
	}

	// Create HTTP server with metrics handler
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metricsHandler(collector))

	ctx, cancel := context.WithCancel(context.Background())
	httpServer := &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		BaseContext:  func(net.Listener) context.Context { return ctx },
	}

	// Create resource sampler if db is provided
	var sampler *resourceSampler
	if db != nil {
		sampler = StartResourceSampler(collector, db)
	}

	return &Server{
		collector:  collector,
		httpServer: httpServer,
		cancel:     cancel,
		sampler:    sampler,
	}
}

// Start begins listening in a background goroutine. Non-blocking.
func (s *Server) Start() error {
	// Check if this is a nop server
	if s.httpServer == nil {
		return nil
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't fail - metrics is optional
			// In production, would use proper logging
		}
	}()

	return nil
}

// Shutdown gracefully stops the server with a 5s timeout.
func (s *Server) Shutdown() {
	// Check if this is a nop server
	if s.httpServer == nil {
		return
	}

	// Stop the sampler
	if s.sampler != nil {
		s.sampler.Stop()
	}

	// Cancel context to stop new requests
	if s.cancel != nil {
		s.cancel()
	}

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.httpServer.Shutdown(ctx)
}

// metricsHandler creates the /metrics HTTP handler.
func metricsHandler(collector Collector) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set Content-Type header
		w.Header().Set("Content-Type", ContentType())

		// Gather and serialize metrics
		families := collector.Gather()
		Serialize(w, families)
	}
}