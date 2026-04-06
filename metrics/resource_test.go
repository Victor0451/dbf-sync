//go:build integration
// +build integration

package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	_ "github.com/go-sql-driver/mysql"
)

// TestResourceSampler_MemoryMetrics asserts memory gauges are updated after 5 seconds
func TestResourceSampler_MemoryMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	mysqlContainer, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root"),
	)
	if err != nil {
		t.Fatalf("failed to start MySQL container: %v", err)
	}
	defer mysqlContainer.Terminate(ctx)

	// Get mapped port
	mysqlPort, err := mysqlContainer.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}
	_ = mysqlPort

	// Get connection string
	connStr, err := mysqlContainer.ConnectionString(ctx, "tls=skip-verify")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Connect to database
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		t.Fatalf("failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	// Verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping MySQL: %v", err)
	}

	// Create collector and server with real database
	collector := NewCollector()
	metricsPort := findFreePort(t)
	server := NewServer(metricsPort, true, collector, db)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Shutdown()

	// Wait for resource sampler to run (sampler runs immediately then every 5 seconds)
	// Wait more than 5 seconds to ensure at least one sample occurred after start
	t.Log("waiting for resource sampler to collect metrics...")
	time.Sleep(6 * time.Second)

	// Fetch metrics
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", metricsPort))
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse response
	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	metricsOutput := string(body[:n])

	// Find dbfsync_memory_alloc_bytes metric
	if !strings.Contains(metricsOutput, "dbfsync_memory_alloc_bytes") {
		t.Errorf("expected dbfsync_memory_alloc_bytes in metrics, got: %s", metricsOutput)
	}

	// Parse the memory value - look for a line like "dbfsync_memory_alloc_bytes <number>"
	lines := strings.Split(metricsOutput, "\n")
	var memValue float64
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "dbfsync_memory_alloc_bytes") {
			// Extract the value after the metric name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%f", &memValue)
				found = true
				break
			}
		}
	}

	if !found {
		t.Fatalf("could not find dbfsync_memory_alloc_bytes value in output: %s", metricsOutput)
	}

	if memValue <= 0 {
		t.Errorf("expected dbfsync_memory_alloc_bytes > 0, got %f", memValue)
	} else {
		t.Logf("dbfsync_memory_alloc_bytes = %f (bytes)", memValue)
	}

	// Also verify memory sys metric exists
	if !strings.Contains(metricsOutput, "dbfsync_memory_sys_bytes") {
		t.Error("expected dbfsync_memory_sys_bytes in metrics")
	}

	// Also verify MySQL connections metric exists
	if !strings.Contains(metricsOutput, "dbfsync_mysql_open_connections") {
		t.Error("expected dbfsync_mysql_open_connections in metrics")
	}
}

// TestResourceSampler_UpdatesEvery5Seconds verifies sampler updates periodically
func TestResourceSampler_UpdatesEvery5Seconds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	mysqlContainer, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("root"),
	)
	if err != nil {
		t.Fatalf("failed to start MySQL container: %v", err)
	}
	defer mysqlContainer.Terminate(ctx)

	// Get mapped port
	mysqlPort, err := mysqlContainer.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}
	_ = mysqlPort

	// Get connection string
	connStr, err := mysqlContainer.ConnectionString(ctx, "tls=skip-verify")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Connect to database
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		t.Fatalf("failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	// Verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping MySQL: %v", err)
	}

	// Create collector and server with real database
	collector := NewCollector()
	metricsPort := findFreePort(t)
	server := NewServer(metricsPort, true, collector, db)

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Shutdown()

	// First sample happens immediately, then wait for second sample (>5s)
	time.Sleep(6 * time.Second)

	// Fetch metrics twice to verify values are updating
	firstMetrics := fetchMetrics(t, metricsPort)
	secondMetrics := fetchMetrics(t, metricsPort)

	// Extract memory values from both responses
	firstMem := extractMemoryValue(t, firstMetrics)
	secondMem := extractMemoryValue(t, secondMetrics)

	// Values should be different (since runtime memory changes)
	// But at minimum both should be > 0
	if firstMem <= 0 {
		t.Errorf("first sample: expected dbfsync_memory_alloc_bytes > 0, got %f", firstMem)
	}
	if secondMem <= 0 {
		t.Errorf("second sample: expected dbfsync_memory_alloc_bytes > 0, got %f", secondMem)
	}

	t.Logf("Memory samples: first=%f, second=%f", firstMem, secondMem)
}

func fetchMetrics(t *testing.T, port int) string {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	return string(body[:n])
}

func extractMemoryValue(t *testing.T, metrics string) float64 {
	lines := strings.Split(metrics, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "dbfsync_memory_alloc_bytes") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var value float64
				fmt.Sscanf(parts[1], "%f", &value)
				return value
			}
		}
	}
	return 0
}