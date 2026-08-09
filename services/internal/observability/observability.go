package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Initialize(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("OCCCCAD_LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level, AddSource: level == slog.LevelDebug,
	})))

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", "demo03"),
		)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(time.Second)))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown, nil
}

func HTTPHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "occccad.http",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))
}

func Shutdown(ctx context.Context, shutdowns ...func(context.Context) error) error {
	var result error
	for index := len(shutdowns) - 1; index >= 0; index-- {
		if shutdowns[index] != nil {
			result = errors.Join(result, shutdowns[index](ctx))
		}
	}
	return result
}
