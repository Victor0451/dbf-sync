package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestInit_TextFormat(t *testing.T) {
	// Capture stderr output
	var buf bytes.Buffer
	originalStderr := os.Stderr
	os.Stderr = &buf

	err := Init("text", "info")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Restore stderr
	os.Stderr = originalStderr

	if DefaultHandler == nil {
		t.Error("DefaultHandler should not be nil after Init")
	}

	// Test that we can log
	slog.Info("test message", "key", "value")

	// Revert to nil for other tests
	DefaultHandler = nil
}

func TestInit_JsonFormat(t *testing.T) {
	var buf bytes.Buffer
	originalStdout := os.Stdout
	os.Stdout = &buf

	err := Init("json", "info")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	os.Stdout = originalStdout

	if DefaultHandler == nil {
		t.Error("DefaultHandler should not be nil after Init")
	}

	// Test that we can log
	slog.Info("test message", "key", "value")

	// Verify JSON output contains our message and key
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected JSON output to contain 'test message', got: %s", output)
	}

	// Reset
	DefaultHandler = nil
}

func TestInit_InvalidFormat(t *testing.T) {
	err := Init("invalid", "info")
	if err == nil {
		t.Error("Expected error for invalid format")
	}

	initErr, ok := err.(*InitError)
	if !ok {
		t.Error("Expected InitError type")
	}
	if initErr.Format != "invalid" {
		t.Errorf("Expected format 'invalid', got: %s", initErr.Format)
	}
}

func TestInit_InvalidLevel(t *testing.T) {
	// Invalid level should not fail - LevelFromString returns slog.LevelInfo for unknown levels
	err := Init("text", "invalid_level")
	if err != nil {
		t.Fatalf("Init should not fail for invalid level: %v", err)
	}

	// Reset
	DefaultHandler = nil
}

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
		{"", slog.LevelInfo},         // default
	}

	for _, tt := range tests {
		result := LevelFromString(tt.input)
		if result != tt.expected {
			t.Errorf("LevelFromString(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestLevelFiltering_Debug(t *testing.T) {
	// Init with debug level - should see all
	err := Init("text", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	// Create a handler that writes to our buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler
	slog.SetDefault(slog.New(handler))

	// Log at different levels
	slog.Debug("debug message", "key", "value")
	slog.Info("info message", "key", "value")
	slog.Warn("warn message", "key", "value")
	slog.Error("error message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("Debug level messages should appear at debug level")
	}

	// Reset
	DefaultHandler = nil
}

func TestLevelFiltering_Info(t *testing.T) {
	// Init with info level - should see info, warn, error but not debug
	err := Init("text", "info")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	DefaultHandler = handler
	slog.SetDefault(slog.New(handler))

	// Log at different levels
	slog.Debug("debug message")
	slog.Info("info message")
	slog.Warn("warn message")
	slog.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("Debug messages should not appear at info level")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Info messages should appear at info level")
	}

	// Reset
	DefaultHandler = nil
}

func TestLevelFiltering_Error(t *testing.T) {
	// Init with error level - should only see error
	err := Init("text", "error")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	DefaultHandler = handler
	slog.SetDefault(slog.New(handler))

	slog.Debug("debug message")
	slog.Info("info message")
	slog.Warn("warn message")
	slog.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("Debug messages should not appear at error level")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info messages should not appear at error level")
	}
	if strings.Contains(output, "warn message") {
		t.Error("Warn messages should not appear at error level")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error messages should appear at error level")
	}

	// Reset
	DefaultHandler = nil
}

func TestStructuredAttributes(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler
	slog.SetDefault(slog.New(handler))

	// Test logging with structured attributes
	slog.Info("sync completed", "database", "testdb", "table", "users", "mode", "upsert")

	// Parse the JSON output
	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Check for our attributes
	if logEntry["database"] != "testdb" {
		t.Errorf("Expected database=testdb, got: %v", logEntry["database"])
	}
	if logEntry["table"] != "users" {
		t.Errorf("Expected table=users, got: %v", logEntry["table"])
	}
	if logEntry["mode"] != "upsert" {
		t.Errorf("Expected mode=upsert, got: %v", logEntry["mode"])
	}

	// Reset
	DefaultHandler = nil
}

func TestLogWithOperation(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler

	ctx := context.Background()
	LogWithOperation(ctx, slog.Info, "Operation started", "mydb", "payments", "append")

	output := buf.String()
	if !strings.Contains(output, "mydb") {
		t.Error("Expected database attribute in output")
	}
	if !strings.Contains(output, "payments") {
		t.Error("Expected table attribute in output")
	}
	if !strings.Contains(output, "append") {
		t.Error("Expected mode attribute in output")
	}

	// Reset
	DefaultHandler = nil
}

func TestLogWithDatabase_Enrichment(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler

	// Test with correlation ID in context
	ctx := WithCorrelationID(context.Background(), "test-correlation-123")
	LogWithDatabase(ctx, slog.Info, "database operation", "testdb")

	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["correlation_id"] != "test-correlation-123" {
		t.Errorf("Expected correlation_id=test-correlation-123, got: %v", logEntry["correlation_id"])
	}
	if logEntry["database"] != "testdb" {
		t.Errorf("Expected database=testdb, got: %v", logEntry["database"])
	}

	// Reset and test without correlation ID
	buf.Reset()
	DefaultHandler = handler
	ctxNoID := context.Background()
	LogWithDatabase(ctxNoID, slog.Info, "database operation", "testdb")

	logEntry = map[string]interface{}{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if _, exists := logEntry["correlation_id"]; exists {
		t.Error("Expected no correlation_id when not in context")
	}
	if logEntry["database"] != "testdb" {
		t.Errorf("Expected database=testdb, got: %v", logEntry["database"])
	}

	DefaultHandler = nil
}

func TestLogWithTable_Enrichment(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler

	// Test with correlation ID in context
	ctx := WithCorrelationID(context.Background(), "table-op-456")
	LogWithTable(ctx, slog.Info, "table operation", "users")

	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["correlation_id"] != "table-op-456" {
		t.Errorf("Expected correlation_id=table-op-456, got: %v", logEntry["correlation_id"])
	}
	if logEntry["table"] != "users" {
		t.Errorf("Expected table=users, got: %v", logEntry["table"])
	}

	// Reset and test without correlation ID
	buf.Reset()
	DefaultHandler = handler
	ctxNoID := context.Background()
	LogWithTable(ctxNoID, slog.Info, "table operation", "users")

	logEntry = map[string]interface{}{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if _, exists := logEntry["correlation_id"]; exists {
		t.Error("Expected no correlation_id when not in context")
	}

	DefaultHandler = nil
}

func TestLogWithOperation_Enrichment(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler

	// Test with correlation ID in context
	ctx := WithCorrelationID(context.Background(), "op-789-abc")
	LogWithOperation(ctx, slog.Info, "sync operation", "mydb", "orders", "upsert")

	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if logEntry["correlation_id"] != "op-789-abc" {
		t.Errorf("Expected correlation_id=op-789-abc, got: %v", logEntry["correlation_id"])
	}
	if logEntry["database"] != "mydb" {
		t.Errorf("Expected database=mydb, got: %v", logEntry["database"])
	}
	if logEntry["table"] != "orders" {
		t.Errorf("Expected table=orders, got: %v", logEntry["table"])
	}
	if logEntry["mode"] != "upsert" {
		t.Errorf("Expected mode=upsert, got: %v", logEntry["mode"])
	}

	// Reset and test without correlation ID
	buf.Reset()
	DefaultHandler = handler
	ctxNoID := context.Background()
	LogWithOperation(ctxNoID, slog.Info, "sync operation", "mydb", "orders", "upsert")

	logEntry = map[string]interface{}{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if _, exists := logEntry["correlation_id"]; exists {
		t.Error("Expected no correlation_id when not in context")
	}

	DefaultHandler = nil
}

func TestLogEnrichment_PreservesExistingAttrs(t *testing.T) {
	err := Init("json", "debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	DefaultHandler = handler

	// Test that existing attributes (database, table, mode) are preserved alongside correlation_id
	ctx := WithCorrelationID(context.Background(), "correlation-preserve-test")
	LogWithOperation(ctx, slog.Info, "operation with preserve", "testdb", "customers", "append")

	var logEntry map[string]interface{}
	err = json.Unmarshal([]byte(buf.String()), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Verify all attributes present
	if logEntry["correlation_id"] != "correlation-preserve-test" {
		t.Errorf("Expected correlation_id, got: %v", logEntry["correlation_id"])
	}
	if logEntry["database"] != "testdb" {
		t.Errorf("Expected database, got: %v", logEntry["database"])
	}
	if logEntry["table"] != "customers" {
		t.Errorf("Expected table, got: %v", logEntry["table"])
	}
	if logEntry["mode"] != "append" {
		t.Errorf("Expected mode, got: %v", logEntry["mode"])
	}

	DefaultHandler = nil
}