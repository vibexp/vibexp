package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// runtimeMetricPoints returns, per Go runtime metric name, its int64 data
// points as exported by the contrib runtime scope.
func runtimeMetricPoints(t *testing.T, rm *metricdata.ResourceMetrics) map[string][]metricdata.DataPoint[int64] {
	t.Helper()
	points := map[string][]metricdata.DataPoint[int64]{}
	for _, sm := range rm.ScopeMetrics {
		if sm.Scope.Name != otelruntime.ScopeName {
			continue
		}
		for _, mtr := range sm.Metrics {
			switch data := mtr.Data.(type) {
			case metricdata.Sum[int64]:
				points[mtr.Name] = data.DataPoints
			case metricdata.Gauge[int64]:
				points[mtr.Name] = data.DataPoints
			default:
				t.Fatalf("runtime metric %s has unexpected data type %T", mtr.Name, mtr.Data)
			}
		}
	}
	return points
}

// TestNew_ExportsGoRuntimeMetrics is the #1277 acceptance criterion: with
// metrics export configured, the exported series include the goroutine count,
// the memory the runtime holds, and the GC's view of the heap.
func TestNew_ExportsGoRuntimeMetrics(t *testing.T) {
	_, reader := newTestMetricsWithReader(t)

	points := runtimeMetricPoints(t, scrapeMetrics(t, reader))

	for _, name := range []string{
		"go.goroutine.count",    // goroutines: the leak signal of #1275
		"go.memory.used",        // memory in use, split stack / other (heap)
		"go.memory.allocated",   // cumulative heap allocation, bytes
		"go.memory.allocations", // cumulative heap allocation, objects
		"go.memory.gc.goal",     // GC: heap size target for the cycle
		"go.config.gogc",        // GC: configured GOGC
		"go.processor.limit",    // GOMAXPROCS
	} {
		assert.NotEmpty(t, points[name], "runtime metric %s is not exported", name)
	}

	// The values are read from the live runtime, not registered and left at zero.
	require.Len(t, points["go.goroutine.count"], 1)
	assert.Positive(t, points["go.goroutine.count"][0].Value)

	require.Len(t, points["go.memory.used"], 2, "go.memory.used reports one series per go.memory.type")
	var memoryTypes []string
	for _, dp := range points["go.memory.used"] {
		assert.Positive(t, dp.Value)
		v, ok := dp.Attributes.Value("go.memory.type")
		require.True(t, ok)
		memoryTypes = append(memoryTypes, v.AsString())
	}
	assert.ElementsMatch(t, []string{"stack", "other"}, memoryTypes)

	require.Len(t, points["go.memory.gc.goal"], 1)
	assert.Positive(t, points["go.memory.gc.goal"][0].Value)
}

// failingMeterProvider hands out meters whose every observable up-down counter
// fails to register, which is the first instrument the runtime package creates.
type failingMeterProvider struct {
	noop.MeterProvider
	err error
}

func (p failingMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return failingMeter{err: p.err}
}

type failingMeter struct {
	noop.Meter
	err error
}

func (m failingMeter) Int64ObservableUpDownCounter(
	string, ...metric.Int64ObservableUpDownCounterOption,
) (metric.Int64ObservableUpDownCounter, error) {
	return nil, m.err
}

func TestStartRuntimeMetrics_ReturnsRegistrationError(t *testing.T) {
	registrationErr := errors.New("instrument rejected")

	err := startRuntimeMetrics(failingMeterProvider{err: registrationErr})

	require.ErrorIs(t, err, registrationErr)
	assert.Contains(t, err.Error(), "failed to start Go runtime metrics")
}

// TestShutdownAfterInitFailure covers New's failure path: the provider is
// released and the initialization error is what the caller sees.
func TestShutdownAfterInitFailure(t *testing.T) {
	initErr := errors.New("init failed")

	t.Run("shutdown succeeds", func(t *testing.T) {
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))

		err := shutdownAfterInitFailure(context.Background(), provider, initErr)

		require.ErrorIs(t, err, initErr)
		assert.Equal(t, initErr, err, "a clean shutdown must not wrap the init error")
		// The provider really was shut down: a second shutdown reports it.
		assert.Error(t, provider.Shutdown(context.Background()))
	})

	t.Run("shutdown also fails", func(t *testing.T) {
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
		require.NoError(t, provider.Shutdown(context.Background()))

		err := shutdownAfterInitFailure(context.Background(), provider, initErr)

		require.ErrorIs(t, err, initErr)
		assert.Contains(t, err.Error(), "failed to shutdown meter provider")
	})
}
