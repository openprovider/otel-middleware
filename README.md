# OpenTelemetry Middleware

A Go library providing OpenTelemetry tracing middleware for gRPC and HTTP services.

## Features

- **gRPC Server Middleware**: Automatic tracing for incoming gRPC requests (unary and streaming)
- **gRPC Client Middleware**: Automatic tracing for outgoing gRPC requests (unary and streaming)
- **HTTP Server Middleware**: Automatic tracing for HTTP requests
- **HTTP Client Instrumentation**: Helper for injecting trace context into outbound HTTP requests
- **Configurable OTLP Exporters**: Support for both HTTP and gRPC OTLP exporters
- **Flexible Sampling**: Configurable trace sampling rates

## Installation

```bash
go get github.com/openprovider/otel-middleware
```

## Usage

### Configuration

```go
import "github.com/openprovider/otel-middleware"

config := &otel.Config{
    Enabled:        true,
    Endpoint:       "https://api.honeycomb.io",
    ServiceName:    "my-service",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    Protocol:       "http/protobuf",
    Headers:        "Authorization=Bearer your-api-key",
    BatchTimeout:   5,
    SamplingRate:   1.0,
}
```

### Initialize Provider

```go
ctx := context.Background()
provider, err := otel.New(ctx, config)
if err != nil {
    log.Fatal(err)
}
defer provider.Shutdown(ctx)
```

### gRPC Server Middleware

```go
server := grpc.NewServer(
    grpc.UnaryInterceptor(otel.GRPCUnaryServerInterceptor()),
    grpc.StreamInterceptor(otel.GRPCStreamServerInterceptor()),
)
```

### gRPC Client Middleware

```go
conn, err := grpc.Dial("localhost:9090",
    grpc.WithUnaryInterceptor(otel.GRPCUnaryClientInterceptor()),
    grpc.WithStreamInterceptor(otel.GRPCStreamClientInterceptor()),
)
```

### HTTP Server Middleware

```go
mux := http.NewServeMux()
mux.HandleFunc("/health", healthHandler)

server := &http.Server{
    Handler: otel.HTTPMiddleware(mux),
}
```

### HTTP Client Instrumentation

```go
req, _ := http.NewRequest("GET", "https://api.example.com", nil)
otel.InjectHTTPHeaders(req)
resp, err := http.DefaultClient.Do(req)
```

## Configuration Options

- **Enabled**: Enable/disable tracing
- **Endpoint**: OTLP collector endpoint
- **ServiceName**: Service name for traces
- **ServiceVersion**: Service version
- **Environment**: Deployment environment
- **Protocol**: `http`, `http/protobuf`, `grpc`, or `grpc/protobuf`
- **Headers**: Authorization headers (format: `Authorization=Bearer token`)
- **BatchTimeout**: Batch export timeout in seconds
- **SamplingRate**: Trace sampling rate (0.0 to 1.0)

## License

Copyright © 2024 Openprovider
