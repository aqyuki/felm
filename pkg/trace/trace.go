package trace

import (
	"context"
	"errors"

	"github.com/rs/xid"
)

type contextKey string

const traceIDKey = contextKey("traceID")

var ErrTraceIDNotFound = errors.New("trace ID not found")

func WithTraceID(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceIDKey, xid.New().String())
}

func AcquireTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}
