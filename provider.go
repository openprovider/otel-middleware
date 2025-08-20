package otel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Provider struct {
	cfg           *Config
	traceProvider *sdktrace.TracerProvider
}

// New initializes OpenTelemetry tracing
func New(ctx context.Context, cfg *Config) (*Provider, error) {
	// Skip if disabled
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	// Validate required configuration
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("OpenTelemetry endpoint must be set")
	}

	var client otlptrace.Client
	var err error

	// Parse API key if provided
	var apiKey string
	if cfg.Headers != "" && strings.Contains(cfg.Headers, "Authorization=") {
		parts := strings.SplitN(cfg.Headers, "=", 2)
		if len(parts) == 2 {
			apiKey = parts[1]
		}
	}

	// Create client based on protocol
	switch cfg.Protocol {
	case "http/protobuf", "http":
		// HTTP exporter
		// For HTTP exporters, we need to remove the protocol scheme
		httpEndpoint := strings.TrimPrefix(cfg.Endpoint, "https://")
		httpEndpoint = strings.TrimPrefix(httpEndpoint, "http://")

		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(httpEndpoint),
		}

		// Handle HTTPS vs HTTP
		if !strings.HasPrefix(cfg.Endpoint, "https://") {
			// Only call WithInsecure for HTTP (not HTTPS)
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		// Add API key header if provided
		if apiKey != "" {
			opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
				"Authorization": apiKey,
			}))
		}

		client = otlptracehttp.NewClient(opts...)

	case "grpc", "grpc/protobuf":
		// gRPC exporter
		// Remove protocol scheme for gRPC
		grpcEndpoint := strings.TrimPrefix(cfg.Endpoint, "https://")
		grpcEndpoint = strings.TrimPrefix(grpcEndpoint, "http://")

		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(grpcEndpoint),
		}

		// Handle TLS for gRPC
		if !strings.HasPrefix(cfg.Endpoint, "https://") {
			// Only call WithInsecure for HTTP (not HTTPS)
			opts = append(opts, otlptracegrpc.WithInsecure())
		}

		// Note: API key for gRPC needs to be handled in metadata context
		// This will be implemented in the propagation layer if needed

		client = otlptracegrpc.NewClient(opts...)

	default:
		return nil, fmt.Errorf("unsupported OTLP protocol: %s", cfg.Protocol)
	}

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler based on sampling rate
	var sampler sdktrace.Sampler
	if cfg.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SamplingRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRate)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Duration(cfg.BatchTimeout)*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator for trace context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		cfg:           cfg,
		traceProvider: tp,
	}, nil
}

// Shutdown processes graceful shutdown for OpenTelemetry
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.traceProvider == nil {
		return nil
	}

	return p.traceProvider.Shutdown(ctx)
}
