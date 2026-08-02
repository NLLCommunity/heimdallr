package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	telemetryScopeName = "github.com/NLLCommunity/heimdallr"
	otelHTTPLogsPath   = "/v1/logs"
	otelHTTPTracesPath = "/v1/traces"
)

type originalRequestContextKey struct{}

var scrubbedTelemetryIPHeaders = []string{
	"X-Forwarded-For",
	"Forwarded",
	"X-Real-IP",
	"True-Client-IP",
	"CF-Connecting-IP",
}

var scrubbedTelemetryFingerprintHeaders = []string{
	"User-Agent",
	"Sec-CH-UA",
	"Sec-CH-UA-Arch",
	"Sec-CH-UA-Bitness",
	"Sec-CH-UA-Full-Version",
	"Sec-CH-UA-Full-Version-List",
	"Sec-CH-UA-Mobile",
	"Sec-CH-UA-Model",
	"Sec-CH-UA-Platform",
	"Sec-CH-UA-Platform-Version",
	"Sec-CH-UA-Wow64",
}

var (
	numericTelemetryRouteSegmentPattern = regexp.MustCompile(`^[0-9]+$`)
	telemetryRouteParamPattern          = regexp.MustCompile(`^\{([^{}]+)\}$`)
)

var allowedTelemetryRouteSegments = map[string]struct{}{
	"anti-spam":   {},
	"audit-log":   {},
	"auditlog":    {},
	"ban-footer":  {},
	"callback":    {},
	"delete":      {},
	"edit":        {},
	"gatekeep":    {},
	"guild":       {},
	"guilds":      {},
	"infractions": {},
	"join-leave":  {},
	"load":        {},
	"login":       {},
	"logout":      {},
	"mod-channel": {},
	"modmail":     {},
	"new":         {},
	"oauth":       {},
	"posts":       {},
	"preview":     {},
	"publish":     {},
	"sandbox":     {},
	"send":        {},
	"settings":    {},
	"start":       {},
	"static":      {},
	"unpublish":   {},
}

type otelRuntime struct {
	logProvider   *sdklog.LoggerProvider
	traceProvider *sdktrace.TracerProvider
	logHandler    slog.Handler
	tracer        trace.Tracer
	restore       func()
}

func startOTel(ctx context.Context, cfg OTelConfig) (*otelRuntime, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceNamespace(cfg.ServiceNamespace),
			semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}

	logOpts := buildOTLPLogOptions(cfg)
	traceOpts := buildOTLPTraceOptions(cfg)

	logExporter, err := otlploghttp.New(ctx, logOpts...)
	if err != nil {
		return nil, fmt.Errorf("otel log exporter: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx, traceOpts...)
	if err != nil {
		_ = logExporter.Shutdown(ctx)
		return nil, fmt.Errorf("otel trace exporter: %w", err)
	}

	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)

	previousLogProvider := global.GetLoggerProvider()
	previousTracerProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()

	global.SetLoggerProvider(logProvider)
	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &otelRuntime{
		logProvider:   logProvider,
		traceProvider: traceProvider,
		logHandler:    buildOTelLogHandler(logProvider),
		tracer:        traceProvider.Tracer(telemetryScopeName),
		restore: func() {
			global.SetLoggerProvider(previousLogProvider)
			otel.SetTracerProvider(previousTracerProvider)
			otel.SetTextMapPropagator(previousPropagator)
		},
	}, nil
}

func buildOTelLogHandler(provider otellog.LoggerProvider) slog.Handler {
	return newOTelRedactingHandler(otelslog.NewHandler(
		telemetryScopeName,
		otelslog.WithLoggerProvider(provider),
	))
}

type otelHTTPEndpoint struct {
	endpoint string
	basePath string
	insecure bool
}

func buildOTLPLogOptions(cfg OTelConfig) []otlploghttp.Option {
	endpoint := otelHTTPEndpointFromConfig(cfg)
	opts := []otlploghttp.Option{
		otlploghttp.WithHeaders(cfg.Headers),
		otlploghttp.WithEndpoint(endpoint.endpoint),
		otlploghttp.WithURLPath(endpoint.signalPath(otelHTTPLogsPath)),
	}
	if endpoint.insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	return opts
}

func buildOTLPTraceOptions(cfg OTelConfig) []otlptracehttp.Option {
	endpoint := otelHTTPEndpointFromConfig(cfg)
	opts := []otlptracehttp.Option{
		otlptracehttp.WithHeaders(cfg.Headers),
		otlptracehttp.WithEndpoint(endpoint.endpoint),
		otlptracehttp.WithURLPath(endpoint.signalPath(otelHTTPTracesPath)),
	}
	if endpoint.insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return opts
}

func otelHTTPEndpointFromConfig(cfg OTelConfig) otelHTTPEndpoint {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	result := otelHTTPEndpoint{
		endpoint: endpoint,
		insecure: cfg.Insecure,
	}

	if !strings.Contains(endpoint, "://") {
		return result
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return result
	}
	result.endpoint = parsed.Host
	result.basePath = normalizeOTelHTTPBasePath(parsed.Path)
	result.insecure = cfg.Insecure || strings.EqualFold(parsed.Scheme, "http")
	return result
}

func normalizeOTelHTTPBasePath(basePath string) string {
	for _, signalPath := range []string{otelHTTPLogsPath, otelHTTPTracesPath} {
		if strings.HasSuffix(basePath, signalPath) {
			basePath = strings.TrimSuffix(basePath, signalPath)
			break
		}
	}
	return basePath
}

func (e otelHTTPEndpoint) signalPath(defaultPath string) string {
	if e.basePath == "" || e.basePath == "/" {
		return defaultPath
	}
	return "/" + strings.TrimLeft(path.Join(e.basePath, defaultPath), "/")
}

func (r *otelRuntime) WrapHTTPHandler(handler http.Handler) http.Handler {
	if handler == nil {
		return nil
	}

	instrumented := otelhttp.NewHandler(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			originalReq, _ := req.Context().Value(originalRequestContextKey{}).(*http.Request)
			if originalReq == nil {
				handler.ServeHTTP(w, req)
				return
			}
			handler.ServeHTTP(w, originalReq.WithContext(req.Context()))
		}),
		"heimdallr.web",
		otelhttp.WithTracerProvider(r.traceProvider),
		otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
			return req.Method + " " + SanitizedHTTPRequestRoute(req)
		}),
	)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		scrubbedReq := scrubRequestForTelemetry(req).WithContext(
			context.WithValue(req.Context(), originalRequestContextKey{}, req),
		)
		instrumented.ServeHTTP(w, scrubbedReq)
	})
}

func (r *otelRuntime) StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return r.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func (r *otelRuntime) Shutdown(ctx context.Context) error {
	var errs []error
	if r.logProvider != nil {
		if err := r.logProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("otel logs shutdown: %w", err))
		}
	}
	if r.traceProvider != nil {
		if err := r.traceProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("otel traces shutdown: %w", err))
		}
	}
	if r.restore != nil {
		r.restore()
	}
	return errors.Join(errs...)
}

func scrubRequestForTelemetry(req *http.Request) *http.Request {
	scrubbedReq := req.Clone(req.Context())
	scrubbedReq.RemoteAddr = ""
	for _, header := range scrubbedTelemetryIPHeaders {
		scrubbedReq.Header.Del(header)
	}
	for _, header := range scrubbedTelemetryFingerprintHeaders {
		scrubbedReq.Header.Del(header)
	}
	route := SanitizedHTTPRequestRoute(req)
	if scrubbedReq.URL != nil {
		scrubbedReq.URL.Path = route
		scrubbedReq.URL.RawPath = route
		scrubbedReq.URL.RawQuery = ""
		scrubbedReq.URL.Fragment = ""
		scrubbedReq.URL.ForceQuery = false
	}
	scrubbedReq.RequestURI = route
	return scrubbedReq
}

func SanitizedHTTPRequestRoute(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "/"
	}
	if route := sanitizedHTTPPatternRoute(req.Pattern); route != "" {
		return route
	}
	return sanitizeHTTPRoutePath(req.URL.Path)
}

func sanitizedHTTPPatternRoute(pattern string) string {
	if _, route, ok := strings.Cut(pattern, " "); ok {
		pattern = route
	}
	if pattern == "" {
		return ""
	}
	if pattern == "/" {
		return "/"
	}

	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return "/"
	}

	segments := strings.Split(trimmed, "/")
	for index, segment := range segments {
		segments[index] = sanitizeHTTPRouteSegment(segment)
	}

	route := "/" + strings.Join(segments, "/")
	if strings.HasSuffix(pattern, "/") {
		return route + "/*"
	}
	return route
}

func sanitizeHTTPRoutePath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	for index, segment := range segments {
		segments[index] = sanitizeHTTPRouteSegment(segment)
	}
	return "/" + strings.Join(segments, "/")
}

func sanitizeHTTPRouteSegment(segment string) string {
	normalized := strings.ToLower(strings.TrimSpace(segment))
	if normalized == "" {
		return "{segment}"
	}
	if matches := telemetryRouteParamPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		if strings.Contains(matches[1], "id") {
			return "{id}"
		}
		return "{param}"
	}
	if _, ok := allowedTelemetryRouteSegments[normalized]; ok {
		return normalized
	}
	if numericTelemetryRouteSegmentPattern.MatchString(normalized) {
		return "{id}"
	}
	return "{segment}"
}
