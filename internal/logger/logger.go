package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// DefaultHandler is the global handler used by the package-level slog functions.
// It is initialized by Init.
var DefaultHandler slog.Handler

// initDefault is a package-level logger that logs directly to stderr.
// Used before Init is called.
var initDefault = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

// Init configures the global logger with the specified format and level.
func Init(format, level string) error {
	output := os.Stderr
	if format == "json" {
		output = os.Stdout
	}
	return InitWithOutput(format, level, output)
}

// InitWithOutput configures the global logger with a custom output writer.
func InitWithOutput(format, level string, output io.Writer) error {
	var handler slog.Handler
	lvl := LevelFromString(level)

	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(output, &slog.HandlerOptions{
			Level: lvl,
		})
	case "text":
		handler = slog.NewTextHandler(output, &slog.HandlerOptions{
			Level: lvl,
		})
	default:
		return &InitError{Format: format, Message: "invalid format"}
	}

	DefaultHandler = handler
	slog.SetDefault(slog.New(handler))
	return nil
}

// LevelFromString converts a string level to slog.Level.
// Returns slog.LevelInfo if the string is not recognized.
func LevelFromString(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// InitError is returned when logger initialization fails.
type InitError struct {
	Format  string
	Message string
}

func (e *InitError) Error() string {
	return e.Message
}

// Helper functions for common logging patterns with structured attributes

// buildAttrs builds the attribute list for logging, adding correlation_id if present in context.
func buildAttrs(baseAttrs []slog.Attr, ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(baseAttrs)+1)
	attrs = append(attrs, baseAttrs...)

	if correlationID := CorrelationIDFrom(ctx); correlationID != "" {
		attrs = append(attrs, slog.Attr{Key: "correlation_id", Value: slog.StringValue(correlationID)})
	}

	return attrs
}

// LogWithDatabase adds database context to a log call.
func LogWithDatabase(ctx context.Context, level slog.Level, msg string, database string, args ...interface{}) {
	baseAttrs := []slog.Attr{{Key: "database", Value: slog.StringValue(database)}}

	if DefaultHandler == nil {
		initDefault.Log(ctx, level, msg, append([]interface{}{"database", database}, args...)...)
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(buildAttrs(baseAttrs, ctx)...)
	DefaultHandler.Handle(ctx, r)
}

// LogWithTable adds table context to a log call.
func LogWithTable(ctx context.Context, level slog.Level, msg string, table string, args ...interface{}) {
	baseAttrs := []slog.Attr{{Key: "table", Value: slog.StringValue(table)}}

	if DefaultHandler == nil {
		initDefault.Log(ctx, level, msg, append([]interface{}{"table", table}, args...)...)
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(buildAttrs(baseAttrs, ctx)...)
	DefaultHandler.Handle(ctx, r)
}

// LogWithOperation adds database and table context to a log call.
func LogWithOperation(ctx context.Context, level slog.Level, msg string, database, table, mode string, args ...interface{}) {
	baseAttrs := []slog.Attr{
		{Key: "database", Value: slog.StringValue(database)},
		{Key: "table", Value: slog.StringValue(table)},
		{Key: "mode", Value: slog.StringValue(mode)},
	}

	if DefaultHandler == nil {
		initDefault.Log(ctx, level, msg, append([]interface{}{
			"database", database,
			"table", table,
			"mode", mode,
		}, args...)...)
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(buildAttrs(baseAttrs, ctx)...)
	DefaultHandler.Handle(ctx, r)
}

// SetOutput sets the output writer for the default handler.
// This is useful for testing or redirecting logs.
func SetOutput(w io.Writer) {
	if DefaultHandler == nil {
		initDefault = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
		return
	}
	// Extract the current log level from DefaultHandler and create a new handler
	// that writes to w at the same level.
	lvl := slog.LevelInfo
	for _, l := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if DefaultHandler.Enabled(nil, l) {
			lvl = l
			break
		}
	}
	var newHandler slog.Handler
	switch DefaultHandler.(type) {
	case *slog.JSONHandler:
		newHandler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	default:
		newHandler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	}
	DefaultHandler = newHandler
	slog.SetDefault(slog.New(newHandler))
}

// GetHandler returns the current handler, or nil if not initialized.
func GetHandler() slog.Handler {
	return DefaultHandler
}