//go:build integration
// +build integration

package metrics

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestServer_MetricsEndpoint returns 200 with valid Prometheus text when enabled
func TestServer_MetricsEndpoint(t *testing.T) {
	// Find a random available port
	port := findFreePort(t)

	// Create server with a collector
	collector := NewCollector()

	// Add some test metrics
	collector.SetGauge(MetricActiveOps, nil, 1)
	collector.IncCounter(MetricRecordsProcessed, map[string]string{"database": "test", "table": "pagos"})

	// Create and start server on random port
	server := NewServer(port, true, collector, nil)
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Shutdown()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Make request
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Check Content-Type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %s", contentType)
	}
	if !strings.Contains(contentType, "version=0.0.4") {
		t.Errorf("expected version 0.0.4 in content type, got %s", contentType)
	}

	// Read body and check for dbfsync_ prefix
	body, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("failed to get metrics body: %v", err)
	}
	defer body.Body.Close()

	// Check that response contains dbfsync_ prefixed metrics
	metrics := make([]byte, 4096)
	n, _ := body.Body.Read(metrics)
	metricsOutput := string(metrics[:n])

	if !strings.Contains(metricsOutput, "dbfsync_") {
		t.Errorf("expected metrics with dbfsync_ prefix, got: %s", metricsOutput)
	}
}

// TestServer_NopServerNoListener verifies no server listens when disabled
func TestServer_NopServerNoListener(t *testing.T) {
	port := findFreePort(t)

	// Create server with disabled flag
	collector := NewCollector()
	server := NewServer(port, false, collector, nil)

	// Start should succeed (no-op)
	if err := server.Start(); err != nil {
		t.Fatalf("failed to start disabled server: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Try to connect - should fail since no server is listening
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/metrics", port))

	// If there's no error, we got a response which means something is listening
	if err == nil {
		resp.Body.Close()
		t.Errorf("expected no server listening on disabled metrics, but got response")
	}
}

// TestServer_PortInUse tests graceful handling when port is already in use
func TestServer_PortInUse(t *testing.T) {
	port := findFreePort(t)

	// Start first server
	collector1 := NewCollector()
	server1 := NewServer(port, true, collector1, nil)
	if err := server1.Start(); err != nil {
		t.Fatalf("failed to start first server: %v", err)
	}
	defer server1.Shutdown()

	time.Sleep(100 * time.Millisecond)

	// Try to start second server on same port - should not panic
	collector2 := NewCollector()
	server2 := NewServer(port, true, collector2, nil)
	err := server2.Start()
	// The second server may fail to start, but it shouldn't panic
	if err != nil && err != http.ErrServerClosed {
		// This is expected behavior - the port is already in use
		t.Logf("expected error on duplicate server start: %v", err)
	}
}

// findFreePort finds a free port on localhost
func findFreePort(t *testing.T) int {
	t.Helper()

	// Create a listener to find an available port
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}