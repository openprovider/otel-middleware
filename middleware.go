package otel

import (
	"context"
	"fmt"
	"log"
	"time"

	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Helper function to check if a parent trace context exists
func hasParentTrace(ctx context.Context) bool {
	spanCtx := trace.SpanContextFromContext(ctx)
	return spanCtx.IsValid() && spanCtx.HasTraceID()
}

// Helper function to check if extracted context has a valid trace
func hasValidTraceContext(extractedCtx context.Context) bool {
	spanCtx := trace.SpanContextFromContext(extractedCtx)
	return spanCtx.IsValid() && spanCtx.HasTraceID()
}

// gRPC Server Interceptor for OpenTelemetry
func GRPCUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract trace context from incoming metadata
		md, _ := metadata.FromIncomingContext(ctx)
		extractedCtx := otel.GetTextMapPropagator().Extract(ctx, metadataTextMapCarrier(md))

		// Only create spans if there's a valid parent trace context
		if !hasValidTraceContext(extractedCtx) {
			return handler(ctx, req)
		}

		// Create span only when parent trace exists
		ctx, span := otel.Tracer("grpc-server").Start(
			extractedCtx,
			info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", "domingo-proxy"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		// Record span attributes
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		if err != nil {
			// Convert gRPC error to OpenTelemetry status
			if st, ok := status.FromError(err); ok {
				span.SetStatus(otelcodes.Error, st.Message())
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(st.Code())),
				)
			} else {
				span.SetStatus(otelcodes.Error, err.Error())
			}
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		return resp, err
	}
}

// gRPC Stream Server Interceptor for OpenTelemetry
func GRPCStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := stream.Context()

		// Extract trace context from incoming metadata
		md, _ := metadata.FromIncomingContext(ctx)
		extractedCtx := otel.GetTextMapPropagator().Extract(ctx, metadataTextMapCarrier(md))

		// Only create spans if there's a valid parent trace context
		if !hasValidTraceContext(extractedCtx) {
			return handler(srv, stream)
		}

		// Create span only when parent trace exists
		ctx, span := otel.Tracer("grpc-server").Start(
			extractedCtx,
			info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", "domingo-proxy"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.Bool("rpc.stream", true),
			),
		)
		defer span.End()

		// Create wrapped stream with trace context
		wrappedStream := &wrappedServerStream{
			ServerStream: stream,
			ctx:          ctx,
		}

		start := time.Now()
		err := handler(srv, wrappedStream)
		duration := time.Since(start)

		// Record span attributes
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		if err != nil {
			if st, ok := status.FromError(err); ok {
				span.SetStatus(otelcodes.Error, st.Message())
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(st.Code())),
				)
			} else {
				span.SetStatus(otelcodes.Error, err.Error())
			}
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		return err
	}
}

// HTTP Middleware for OpenTelemetry
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from headers
		extractedCtx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		
		// Only create spans if there's a valid parent trace context
		if !hasValidTraceContext(extractedCtx) {
			next.ServeHTTP(w, r)
			return
		}
		
		// Create span only when parent trace exists
		ctx, span := otel.Tracer("http-server").Start(
			extractedCtx,
			fmt.Sprintf("%s %s", r.Method, r.URL.Path),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.request_id", r.Header.Get("X-Request-ID")),
			),
		)
		defer span.End()

		// Add trace context to request
		r = r.WithContext(ctx)

		// Create response writer wrapper to capture status code
		wrappedWriter := &responseWriter{ResponseWriter: w, statusCode: 200}

		start := time.Now()
		next.ServeHTTP(wrappedWriter, r)
		duration := time.Since(start)

		// Record span attributes
		span.SetAttributes(
			attribute.Int("http.status_code", wrappedWriter.statusCode),
			attribute.Int64("http.duration_ms", duration.Milliseconds()),
		)

		// Set span status based on HTTP status code
		if wrappedWriter.statusCode >= 400 {
			span.SetStatus(otelcodes.Error, fmt.Sprintf("HTTP %d", wrappedWriter.statusCode))
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}
	})
}

// Helper types for middleware
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Metadata text map carrier for gRPC metadata
type metadataTextMapCarrier metadata.MD

func (m metadataTextMapCarrier) Get(key string) string {
	// Convert metadata.MD to access the underlying map
	md := metadata.MD(m)
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m metadataTextMapCarrier) Set(key, value string) {
	// Convert metadata.MD to access the underlying map
	md := metadata.MD(m)
	md.Set(key, value)
}

func (m metadataTextMapCarrier) Keys() []string {
	// Convert metadata.MD to access the underlying map
	md := metadata.MD(m)
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	return keys
}

// gRPC Client Interceptor for OpenTelemetry - INJECT trace context for outbound calls
func GRPCUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// Only create spans and inject context if there's an active trace
		if !hasParentTrace(ctx) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		// Create span for outbound call
		ctx, span := otel.Tracer("grpc-client").Start(
			ctx,
			method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", "domingo-proxy-client"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.target", cc.Target()),
			),
		)
		defer span.End()

		// Inject trace context into outbound metadata
		md := metadata.New(nil)
		otel.GetTextMapPropagator().Inject(ctx, metadataTextMapCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		// Record span attributes
		span.SetAttributes(
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		if err != nil {
			if st, ok := status.FromError(err); ok {
				span.SetStatus(otelcodes.Error, st.Message())
				span.SetAttributes(
					attribute.Int("rpc.grpc.status_code", int(st.Code())),
				)
			} else {
				span.SetStatus(otelcodes.Error, err.Error())
			}
		} else {
			span.SetStatus(otelcodes.Ok, "")
		}

		return err
	}
}

// gRPC Stream Client Interceptor for OpenTelemetry - INJECT trace context for outbound calls
func GRPCStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Only create spans and inject context if there's an active trace
		if !hasParentTrace(ctx) {
			return streamer(ctx, desc, cc, method, opts...)
		}

		// Create span for outbound call
		ctx, span := otel.Tracer("grpc-client").Start(
			ctx,
			method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.service", "domingo-proxy-client"),
				attribute.String("rpc.method", method),
				attribute.String("rpc.target", cc.Target()),
				attribute.Bool("rpc.stream", true),
			),
		)

		// Inject trace context into outbound metadata
		md := metadata.New(nil)
		otel.GetTextMapPropagator().Inject(ctx, metadataTextMapCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			span.SetStatus(otelcodes.Error, err.Error())
			span.End()
			return nil, err
		}

		// Wrap the stream to end span when stream is done
		wrappedStream := &wrappedClientStream{
			ClientStream: stream,
			span:         span,
		}

		return wrappedStream, nil
	}
}

// HTTP Client instrumentation helper
func InjectHTTPHeaders(req *http.Request) {
	// Only inject trace context if there's an active trace
	if !hasParentTrace(req.Context()) {
		return
	}
	
	// Inject trace context into HTTP headers
	otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
}

// Helper for client stream
type wrappedClientStream struct {
	grpc.ClientStream
	span trace.Span
}

func (w *wrappedClientStream) CloseSend() error {
	err := w.ClientStream.CloseSend()
	if err != nil {
		w.span.SetStatus(otelcodes.Error, err.Error())
	} else {
		w.span.SetStatus(otelcodes.Ok, "")
	}
	w.span.End()
	return err
}