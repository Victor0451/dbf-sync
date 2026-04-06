package logger

import (
	"context"
	"testing"
)

func TestWithCorrelationID_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Test round-trip: set then get
	ctx = WithCorrelationID(ctx, "test-correlation-id-123")
	id := CorrelationIDFrom(ctx)

	if id != "test-correlation-id-123" {
		t.Errorf("expected correlation ID 'test-correlation-id-123', got '%s'", id)
	}
}

func TestCorrelationIDFrom_MissingKey(t *testing.T) {
	ctx := context.Background()

	// Without setting any correlation ID, should return empty string
	id := CorrelationIDFrom(ctx)

	if id != "" {
		t.Errorf("expected empty string, got '%s'", id)
	}
}

func TestCorrelationIDFrom_NilContext(t *testing.T) {
	// Should not panic on nil context
	id := CorrelationIDFrom(nil)

	if id != "" {
		t.Errorf("expected empty string on nil context, got '%s'", id)
	}
}

func TestWithCorrelationID_MultipleIDs(t *testing.T) {
	ctx := context.Background()

	// First ID
	ctx = WithCorrelationID(ctx, "first-id")
	first := CorrelationIDFrom(ctx)

	// Override with second ID
	ctx = WithCorrelationID(ctx, "second-id")
	second := CorrelationIDFrom(ctx)

	if first != "first-id" {
		t.Errorf("expected first ID 'first-id', got '%s'", first)
	}
	if second != "second-id" {
		t.Errorf("expected second ID 'second-id', got '%s'", second)
	}
}

func TestCorrelationIDFrom_EmptyContext(t *testing.T) {
	// Empty context should return empty string
	ctx := context.Background()
	id := CorrelationIDFrom(ctx)

	if id != "" {
		t.Errorf("expected empty string, got '%s'", id)
	}
}

func TestWithCorrelationID_EmptyID(t *testing.T) {
	// Setting empty ID should work (edge case)
	ctx := context.Background()
	ctx = WithCorrelationID(ctx, "")
	id := CorrelationIDFrom(ctx)

	if id != "" {
		t.Errorf("expected empty string, got '%s'", id)
	}
}

func TestCorrelationIDFrom_DifferentContextTypes(t *testing.T) {
	// Test with background context
	bg := context.Background()
	if CorrelationIDFrom(bg) != "" {
		t.Error("expected empty from background context")
	}

	// Test with todo context
	todo := context.TODO()
	if CorrelationIDFrom(todo) != "" {
		t.Error("expected empty from TODO context")
	}

	// Test with context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if CorrelationIDFrom(ctx) != "" {
		t.Error("expected empty from cancelled context")
	}

	// Now add correlation ID and verify it works
	ctx = WithCorrelationID(ctx, "test-id")
	if CorrelationIDFrom(ctx) != "test-id" {
		t.Error("correlation ID should be retrievable from cancelled context")
	}
}