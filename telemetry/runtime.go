package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type shutdowner interface {
	Shutdown(context.Context) error
}

type Runtime struct {
	logger      *slog.Logger
	events      Events
	otel        *otelRuntime
	shutdowners []shutdowner
	restore     func()
}

var (
	defaultRuntimeMu         sync.RWMutex
	defaultRuntime           *Runtime
	defaultRuntimeGeneration uint64
)

func Start(ctx context.Context, cfg Config, baseHandler slog.Handler) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if baseHandler == nil {
		baseHandler = slog.Default().Handler()
	}
	baseHandler = newRedactingHandler(baseHandler)

	runtime := &Runtime{
		events: NewNoopEvents(),
	}
	handlers := []slog.Handler{baseHandler}

	if cfg.OTel.Enabled {
		otelRuntime, err := startOTel(ctx, cfg.OTel)
		if err != nil {
			return nil, err
		}
		runtime.otel = otelRuntime
		runtime.shutdowners = append(runtime.shutdowners, otelRuntime)
		handlers = append(handlers, otelRuntime.logHandler)
	}

	if cfg.PostHog.Enabled {
		events, shutdowner, err := startPostHog(cfg.PostHog, slog.New(baseHandler))
		if err != nil {
			if shutdownErr := runtime.Shutdown(ctx); shutdownErr != nil {
				return nil, errors.Join(err, shutdownErr)
			}
			return nil, err
		}
		runtime.events = events
		runtime.shutdowners = append(runtime.shutdowners, shutdowner)
	}

	runtime.logger = slog.New(newFanoutHandler(handlers...))
	if posthogEvents, ok := runtime.events.(*posthogEvents); ok {
		posthogEvents.logger = runtime.logger
	}
	restoreRuntime := SetDefaultRuntime(runtime)
	restoreEvents := SetDefaultEvents(runtime.events)
	runtime.restore = func() {
		restoreEvents()
		restoreRuntime()
	}
	return runtime, nil
}

func (r *Runtime) Logger() *slog.Logger {
	if r == nil || r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

func (r *Runtime) Events() Events {
	if r == nil || r.events == nil {
		return NewNoopEvents()
	}
	return r.events
}

func (r *Runtime) WrapHTTPHandler(handler http.Handler) http.Handler {
	if r == nil || r.otel == nil || handler == nil {
		return handler
	}
	return r.otel.WrapHTTPHandler(handler)
}

func (r *Runtime) StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if r == nil || r.otel == nil {
		return ctx, noop.Span{}
	}
	return r.otel.StartSpan(ctx, name, attrs...)
}

func SetDefaultRuntime(runtime *Runtime) func() {
	defaultRuntimeMu.Lock()
	previous := defaultRuntime
	previousGeneration := defaultRuntimeGeneration
	defaultRuntimeGeneration++
	generation := defaultRuntimeGeneration
	defaultRuntime = runtime
	defaultRuntimeMu.Unlock()

	restored := false

	return func() {
		defaultRuntimeMu.Lock()
		defer defaultRuntimeMu.Unlock()

		if restored {
			return
		}
		restored = true
		if defaultRuntimeGeneration != generation {
			return
		}

		defaultRuntime = previous
		defaultRuntimeGeneration = previousGeneration
	}
}

func StartDefaultSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}

	defaultRuntimeMu.RLock()
	runtime := defaultRuntime
	defaultRuntimeMu.RUnlock()

	if runtime == nil {
		return ctx, noop.Span{}
	}
	return runtime.StartSpan(ctx, name, attrs...)
}

func NormalizeTelemetryToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	var token strings.Builder
	previousSeparator := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			token.WriteRune(char)
			previousSeparator = false
		case char == '.', char == ':', char == '-':
			if token.Len() == 0 || previousSeparator {
				continue
			}
			token.WriteRune(char)
			previousSeparator = true
		default:
			if token.Len() == 0 || previousSeparator {
				continue
			}
			token.WriteByte('_')
			previousSeparator = true
		}
	}

	normalized := strings.Trim(token.String(), "_.:-")
	if normalized == "" || !safeTelemetryTokenPattern.MatchString(normalized) {
		return ""
	}
	return normalized
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}

	var errs []error
	for _, shutdowner := range r.shutdowners {
		if err := shutdowner.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if r.restore != nil {
		r.restore()
	}
	return errors.Join(errs...)
}
