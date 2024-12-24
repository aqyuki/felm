package trace

import (
	"context"
	"testing"
)

func TestTraceID(t *testing.T) {
	t.Parallel()

	t.Run("expect to return a new context with a trace ID", func(t *testing.T) {
		t.Parallel()

		ctx := WithTraceID(context.Background())
		traceID := AcquireTraceID(ctx)

		if traceID == "" {
			t.Errorf("expected trace ID to be not empty, but got empty")
		}
	})

	t.Run("expect to return a empty string when trace ID is not found", func(t *testing.T) {
		t.Parallel()

		traceID := AcquireTraceID(context.Background())

		if traceID != "" {
			t.Errorf("expected trace ID to be empty, but got %s", traceID)
		}
	})
}
