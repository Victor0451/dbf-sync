package logger

import "context"

// correlationIDKey is an unexported type for context key to avoid collisions.
type correlationIDKey struct{}

// WithCorrelationID returns a new context with the correlation ID set.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}

// CorrelationIDFrom extracts the correlation ID from context.
// Returns empty string if not set.
func CorrelationIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return id
	}
	return ""
}