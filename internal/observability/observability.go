// Package observability centralizes logging, panic recovery, and metrics
// helpers.
package observability

import (
	"log/slog"
	"os"
	"runtime/debug"
)

// Nop returns a discard logger for tests.
func Nop() *slog.Logger { return slog.New(slog.NewJSONHandler(discard{}, nil)) }

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// NewLogger builds the structured JSON root logger.
func NewLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

// CatchPanic converts a panic in a long-lived goroutine into a log line so a
// single bad job cannot take down the scheduler or worker pool.
func CatchPanic(log *slog.Logger) {
	if r := recover(); r != nil {
		log.Error("panic recovered", "panic", r, "stack", string(debug.Stack()))
	}
}
