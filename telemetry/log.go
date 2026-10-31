package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger is ukubi's log (mountain plan/15 §7): one JSON object per line on w (an app's stdout), with slog's time,
// level and msg, and trace_id and span_id when the call passes a context holding a span (slog's *Context calls).
// An app installs it with slog.SetDefault, which also routes the standard library's log package through it, at INFO:
// log.Fatal and log.Panic too (an app gives those their level first), with log's prefix and any flags set after the
// install inside msg (install it after log.SetFlags, or drop the call). Any valid span adds its ids, sampled or not.
// Under a logger's WithGroup the ids land inside the group, as every attribute of the record does: the apps open none.
func NewLogger(w io.Writer) *slog.Logger { return NewLoggerAt(w, nil) }

// NewLoggerAt is NewLogger with a minimum level (nil: INFO), for an app's LOG_LEVEL.
func NewLoggerAt(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(traceHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// traceHandler adds the span's ids to a record whose context holds one.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(as)}
}
func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
