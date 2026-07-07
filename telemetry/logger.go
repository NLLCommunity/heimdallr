package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
)

const redactedValue = "[REDACTED]"

func buildTextHandler(w io.Writer, level slog.Level) slog.Handler {
	return newRedactingHandler(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
}

type redactionMode uint8

const (
	redactionModeDefault redactionMode = iota
	redactionModeOTelExport
)

type fanoutHandler struct {
	handlers []slog.Handler
}

func newFanoutHandler(handlers ...slog.Handler) slog.Handler {
	enabled := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			enabled = append(enabled, handler)
		}
	}
	return &fanoutHandler{handlers: enabled}
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithAttrs(attrs))
	}
	return &fanoutHandler{handlers: next}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithGroup(name))
	}
	return &fanoutHandler{handlers: next}
}

type redactingHandler struct {
	next slog.Handler
	mode redactionMode
}

func newRedactingHandler(next slog.Handler) slog.Handler {
	return newRedactingHandlerWithMode(next, redactionModeDefault)
}

func newOTelRedactingHandler(next slog.Handler) slog.Handler {
	return newRedactingHandlerWithMode(next, redactionModeOTelExport)
}

func newRedactingHandlerWithMode(next slog.Handler, mode redactionMode) slog.Handler {
	if next == nil {
		return nil
	}
	return &redactingHandler{next: next, mode: mode}
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr, h.mode))
		return true
	})
	return h.next.Handle(ctx, redacted)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactAttr(attr, h.mode))
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted), mode: h.mode}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name), mode: h.mode}
}

func redactAttr(attr slog.Attr, mode redactionMode) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if shouldRedactLogKey(attr.Key, mode) {
		return slog.String(attr.Key, redactedValue)
	}
	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}

	group := attr.Value.Group()
	redacted := make([]any, 0, len(group))
	for _, child := range group {
		redacted = append(redacted, redactAttr(child, mode))
	}
	return slog.Group(attr.Key, redacted...)
}

func shouldRedactLogKey(key string, mode redactionMode) bool {
	normalized := normalizeLogKey(key)
	if normalized == "" {
		return false
	}
	if isSensitiveNormalizedLogKey(normalized) {
		return true
	}
	return mode == redactionModeOTelExport && isOTelOperationalLogKey(normalized)
}

func normalizeLogKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func isSensitiveNormalizedLogKey(normalized string) bool {
	if isBlockedPropertyKey(normalized) {
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "authorization")
}

func isOTelOperationalLogKey(normalized string) bool {
	switch normalized {
	case "err", "error", "panic", "stack":
		return true
	default:
		return false
	}
}
