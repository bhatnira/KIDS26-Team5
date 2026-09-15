package log

import (
	"context"

	"go.uber.org/zap"
)

type ctxKey struct{}

// WithContext returns a new context carrying the given logger. Used by
// middleware to attach a request-enriched logger to the request context.
func WithContext(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromCtx extracts the logger from context, falling back to the global logger
// if none is attached. Always returns a usable logger — never nil.
func FromCtx(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return L()
	}
	if logger, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return logger
	}
	return L()
}
