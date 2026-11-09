// Package operationlog writes service observations at the HTTP, MCP and
// background-task boundaries. It does not change an operation's result.
package operationlog

import (
	"context"
	"log/slog"

	"github.com/iml885203/code-x-artifacts/internal/service"
)

// Context gives this operation and its nested calls the caller's log sink.
func Context(ctx context.Context, logger *slog.Logger) context.Context {
	return service.WithObservations(ctx, func(o service.Observation) {
		level := slog.LevelInfo
		switch o.Severity {
		case service.Information:
		case service.Warning:
			level = slog.LevelWarn
		case service.Failure:
			level = slog.LevelError
		}
		logger.Log(ctx, level, o.Message, o.Fields...)
	})
}
