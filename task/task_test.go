package task

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/NLLCommunity/heimdallr/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestNew(t *testing.T) {
	execFunc := func(ctx context.Context) {}
	testKey := ContextKey("test_key")

	contextValues := ContextKeyMap{
		testKey: "test_value",
	}
	interval := time.Second

	task := New("test-task", execFunc, contextValues, interval, false)

	assert.NotNil(t, task)
	assert.Equal(t, TaskStatusNotStarted, task.Status())
}

func TestTaskExecution(t *testing.T) {
	var counter int
	var mu sync.Mutex

	execFunc := func(ctx context.Context) {
		mu.Lock()
		counter++
		mu.Unlock()
	}

	task := New("test-task", execFunc, nil, 50*time.Millisecond, false)

	// Test that task starts.
	task.Start()
	assert.Equal(t, TaskStatusRunning, task.Status())

	// Wait for at least 2 executions.
	time.Sleep(125 * time.Millisecond)

	// Stop the task.
	task.Stop()
	assert.Equal(t, TaskStatusStopped, task.Status())

	// Get final counter value.
	mu.Lock()
	finalCounter := counter
	mu.Unlock()

	// Should have executed at least 2 times.
	assert.GreaterOrEqual(t, finalCounter, 2)

	// Wait a bit more and ensure it doesn't continue executing.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	stoppedCounter := counter
	mu.Unlock()

	// Counter should not have increased after stopping.
	assert.Equal(t, finalCounter, stoppedCounter)
}

func TestTaskStartNoWait(t *testing.T) {
	var execCount int
	var mu sync.Mutex

	execFunc := func(ctx context.Context) {
		mu.Lock()
		execCount++
		mu.Unlock()
	}

	task := New("test-task", execFunc, nil, time.Second, false)

	// StartNoWait should execute immediately and then start the timer.
	task.StartNoWait()

	// Give it a moment to execute the immediate call.
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	immediateCount := execCount
	mu.Unlock()

	// Should have executed once immediately.
	assert.Equal(t, 1, immediateCount)
	assert.Equal(t, TaskStatusRunning, task.Status())

	// Clean up.
	task.Stop()
}

func TestTaskContextValues(t *testing.T) {
	testKey := ContextKey("test_key")

	expectedValue := "test_value"
	var actualValue string
	var mu sync.Mutex

	execFunc := func(ctx context.Context) {
		if val := ctx.Value(testKey); val != nil {
			mu.Lock()
			actualValue = val.(string)
			mu.Unlock()
		}
	}

	contextValues := ContextKeyMap{
		testKey: expectedValue,
	}

	task := New("test-task", execFunc, contextValues, 100*time.Millisecond, false)
	task.Start()

	// Wait for execution.
	time.Sleep(150 * time.Millisecond)
	task.Stop()

	mu.Lock()
	result := actualValue
	mu.Unlock()

	assert.Equal(t, expectedValue, result)
}

func TestTaskCancellation(t *testing.T) {
	execFunc := func(ctx context.Context) {
		// Just a simple execution function for this test.
	}

	task := New("test-task", execFunc, nil, 50*time.Millisecond, false)
	task.Start()

	// Let it run briefly.
	time.Sleep(25 * time.Millisecond)

	// Stop the task.
	task.Stop()

	// The context should be cancelled after stopping. This test checks that the cancellation
	// mechanism works, though the specific timing may vary.
	assert.Equal(t, TaskStatusStopped, task.Status())
}

func TestMultipleTaskInstances(t *testing.T) {
	var counter1, counter2 int
	var mu1, mu2 sync.Mutex

	exec1 := func(ctx context.Context) {
		mu1.Lock()
		counter1++
		mu1.Unlock()
	}

	exec2 := func(ctx context.Context) {
		mu2.Lock()
		counter2++
		mu2.Unlock()
	}

	task1 := New("task-1", exec1, nil, 50*time.Millisecond, false)
	task2 := New("task-2", exec2, nil, 75*time.Millisecond, false)

	task1.Start()
	task2.Start()

	time.Sleep(200 * time.Millisecond)

	task1.Stop()
	task2.Stop()

	mu1.Lock()
	final1 := counter1
	mu1.Unlock()

	mu2.Lock()
	final2 := counter2
	mu2.Unlock()

	// Both tasks should have executed independently.
	assert.Greater(t, final1, 0)
	assert.Greater(t, final2, 0)

	// task1 should have executed more times due to shorter interval.
	assert.Greater(t, final1, final2)
}

func TestTaskStatusTransitions(t *testing.T) {
	execFunc := func(ctx context.Context) {}

	task := New("test-task", execFunc, nil, time.Second, false)

	// Initial status.
	assert.Equal(t, TaskStatusNotStarted, task.Status())

	// After starting.
	task.Start()
	assert.Equal(t, TaskStatusRunning, task.Status())

	// After stopping.
	task.Stop()
	assert.Equal(t, TaskStatusStopped, task.Status())
}

// A panic in the exec function must be recovered so it neither crashes the
// process nor stops the ticker. Without the recover in runExec, StartNoWait's
// synchronous first call alone would take down the test process.
func TestTaskRecoversFromPanic(t *testing.T) {
	var calls int
	var mu sync.Mutex

	execFunc := func(ctx context.Context) {
		mu.Lock()
		calls++
		mu.Unlock()
		panic("boom")
	}

	task := New("panic-task", execFunc, nil, 25*time.Millisecond, false)

	// StartNoWait runs exec synchronously first; if the panic escaped, this
	// line would crash the test binary rather than return.
	task.StartNoWait()
	assert.Equal(t, TaskStatusRunning, task.Status())

	// The ticker keeps firing despite each run panicking.
	time.Sleep(90 * time.Millisecond)
	task.Stop()

	mu.Lock()
	got := calls
	mu.Unlock()

	assert.GreaterOrEqual(t, got, 2, "task should keep running after a panic")
}

func TestRunExecLogsCompletionDuration(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	task := &taskImpl{
		name:    "Prune Audit Log",
		exec:    func(context.Context) {},
		context: context.Background(),
	}

	task.runExec()

	output := buf.String()
	assert.Contains(t, output, "task completed")
	assert.Contains(t, output, "task=prune_audit_log")
	assert.NotContains(t, output, "task=\"Prune Audit Log\"")
	assert.Contains(t, output, "duration=")
}

func TestRunExecCancelledLogsOutcomeAndSetsSpanAttribute(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})
	restoreRuntime := installDefaultTelemetryRuntime(t, traceProvider.Tracer("github.com/NLLCommunity/heimdallr"))
	t.Cleanup(restoreRuntime)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})

	task := &taskImpl{
		name: "Cancelable Task",
		exec: func(ctx context.Context) {
			close(started)
			<-ctx.Done()
		},
		context: ctx,
	}

	go func() {
		defer close(done)
		task.runExec()
	}()

	<-started
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelled task run")
	}

	output := buf.String()
	assert.Contains(t, output, "task completed")
	assert.Contains(t, output, "task=cancelable_task")
	assert.Contains(t, output, "outcome=cancelled")
	assert.NotContains(t, output, "outcome=success")

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	attrs := map[string]string{}
	for _, attr := range spans[0].Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsString()
	}
	assert.Equal(t, "cancelable_task", attrs["task.name"])
	assert.Equal(t, "cancelled", attrs["task.outcome"])
}

func TestRunExecPanicLogIncludesDuration(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	task := &taskImpl{
		name: "Panic Task",
		exec: func(context.Context) {
			panic("boom")
		},
		context: context.Background(),
	}

	require.NotPanics(t, task.runExec)

	output := buf.String()
	assert.Contains(t, output, "recovered from panic in scheduled task")
	assert.Contains(t, output, "task=panic_task")
	assert.NotContains(t, output, "task=\"Panic Task\"")
	assert.Contains(t, output, "duration=")
	assert.Contains(t, output, "panic_recovered=true")
	assert.Contains(t, output, "outcome=panic")
	assert.NotContains(t, output, "boom")
	assert.NotContains(t, output, "goroutine")
}

func TestTaskLifecycleLogsUseNormalizedTaskToken(t *testing.T) {
	var buf lockedBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	task := New("Nightly Member Sync", func(context.Context) {}, nil, 15*time.Millisecond, false)

	task.Start()
	time.Sleep(35 * time.Millisecond)
	task.Stop()
	time.Sleep(20 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "task running")
	assert.Contains(t, output, "task stopped")
	assert.Contains(t, output, "task=nightly_member_sync")
	assert.NotContains(t, output, "task=\"Nightly Member Sync\"")
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunExecPassesSpanContextToExec(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
	})
	restoreRuntime := installDefaultTelemetryRuntime(t, traceProvider.Tracer("github.com/NLLCommunity/heimdallr"))
	t.Cleanup(restoreRuntime)

	parentCtx, parentSpan := traceProvider.Tracer("task-test").Start(context.Background(), "parent")
	defer parentSpan.End()

	var spanContext trace.SpanContext
	task := &taskImpl{
		name: "Nightly Member Sync",
		exec: func(ctx context.Context) {
			spanContext = trace.SpanContextFromContext(ctx)
		},
		context: parentCtx,
	}

	task.runExec()

	require.True(t, spanContext.IsValid(), "expected exec context to carry a valid span")
	assert.Equal(t, trace.SpanContextFromContext(parentCtx).TraceID(), spanContext.TraceID())
	assert.NotEqual(t, trace.SpanContextFromContext(parentCtx).SpanID(), spanContext.SpanID())

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "scheduled_task.run", spans[0].Name())
	assert.Equal(t, trace.SpanContextFromContext(parentCtx).SpanID(), spans[0].Parent().SpanID())
	assert.Equal(t, spanContext.SpanID(), spans[0].SpanContext().SpanID())
}

func installDefaultTelemetryRuntime(t *testing.T, tracer trace.Tracer) func() {
	t.Helper()

	runtime := &telemetry.Runtime{}
	runtimeValue := reflect.ValueOf(runtime).Elem()
	otelField := runtimeValue.FieldByName("otel")
	otelValue := reflect.New(otelField.Type().Elem())
	tracerField := otelValue.Elem().FieldByName("tracer")

	reflect.NewAt(tracerField.Type(), unsafe.Pointer(tracerField.UnsafeAddr())).Elem().Set(reflect.ValueOf(tracer))
	reflect.NewAt(otelField.Type(), unsafe.Pointer(otelField.UnsafeAddr())).Elem().Set(otelValue)

	return telemetry.SetDefaultRuntime(runtime)
}
