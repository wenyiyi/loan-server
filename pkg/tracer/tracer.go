package tracer

import (
	applog "LoanService/pkg/log"
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// InitTracer configures the global provider and W3C header propagation.
// An empty endpoint keeps tracing local without exporting. Use an http:// URL
// for a plaintext local collector or https:// for TLS.
func InitTracer(ctx context.Context, endpoint, name string) (func(context.Context) error, error) {
	if name == "" {
		name = "amartha-loan-service"
	}
	options := []trace.TracerProviderOption{
		trace.WithResource(resource.NewSchemaless(attribute.String("service.name", name))),
		trace.WithSampler(trace.ParentBased(trace.AlwaysSample())),
	}
	if endpoint != "" {
		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint))
		if err != nil {
			applog.Error(ctx, "tracer.InitTracer failed", err)
			return nil, err
		}
		options = append(options, trace.WithBatcher(exporter))
	}
	provider := trace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown, nil
}
