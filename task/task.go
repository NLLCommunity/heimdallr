package task

import (
	"context"
	"log/slog"
	"time"

	"github.com/NLLCommunity/heimdallr/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

type TaskStatus int

const (
	TaskStatusNotStarted TaskStatus = iota
	TaskStatusRunning
	TaskStatusStopped
)

type ContextKey string
type ContextKeyMap map[ContextKey]any

const (
	ContextKeyBotClientRef ContextKey = "botClientRef"
)

type Task interface {
	Start()
	StartNoWait()
	Stop()
	Status() TaskStatus
}

var _ Task = (*taskImpl)(nil)

type taskImpl struct {
	name       string
	exec       func(ctx context.Context)
	context    context.Context
	cancelFunc context.CancelFunc
	interval   time.Duration
	taskStatus TaskStatus
	silent     bool
}

func New(name string, exec func(ctx context.Context), contextValues map[ContextKey]any, interval time.Duration, silent bool) Task {
	ctx := context.Background()
	for k, v := range contextValues {
		//nolint:staticcheck
		ctx = context.WithValue(ctx, k, v)
	}
	ctx, cancelFunc := context.WithCancel(ctx)

	return &taskImpl{
		name:       name,
		exec:       exec,
		context:    ctx,
		cancelFunc: cancelFunc,
		interval:   interval,
		taskStatus: TaskStatusNotStarted,
		silent:     silent,
	}
}

func (t *taskImpl) Start() {
	ticker := time.NewTicker(t.interval)
	t.taskStatus = TaskStatusRunning
	taskToken := normalizedTaskToken(t.name)

	go func() {
		for {
			select {
			case <-t.context.Done():
				slog.Info("task stopped", taskLogFields(taskToken)...)
				ticker.Stop()
				return
			case <-ticker.C:
				if !t.silent {
					slog.Info("task running", taskLogFields(taskToken)...)
				}
				t.runExec()
			}
		}
	}()
}

func (t *taskImpl) StartNoWait() {
	taskToken := normalizedTaskToken(t.name)
	if !t.silent {
		slog.Info("task running early", taskLogFields(taskToken)...)
	}
	t.runExec()
	t.Start()
}

// runExec runs the task's exec function, recovering any panic so that a bug in a
// periodic task logs and skips the run instead of crashing the whole process.
func (t *taskImpl) runExec() {
	start := time.Now()
	taskToken := normalizedTaskToken(t.name)
	ctx, span := telemetry.StartDefaultSpan(t.context, "scheduled_task.run", taskSpanAttributes(taskToken)...)
	defer span.End()

	defer func() {
		outcome := "success"
		if r := recover(); r != nil {
			outcome = "panic"
			span.SetAttributes(attribute.String("task.outcome", outcome))
			slog.ErrorContext(
				ctx,
				"recovered from panic in scheduled task",
				append(taskRunLogFields(taskToken, time.Since(start), outcome), "panic_recovered", true)...,
			)
			return
		}
		if ctx.Err() != nil {
			outcome = "cancelled"
		}
		span.SetAttributes(attribute.String("task.outcome", outcome))
		if !t.silent {
			slog.InfoContext(ctx, "task completed", taskRunLogFields(taskToken, time.Since(start), outcome)...)
		}
	}()
	t.exec(ctx)
}

func (t *taskImpl) Stop() {
	t.cancelFunc()
	t.taskStatus = TaskStatusStopped
}

func (t *taskImpl) Status() TaskStatus {
	return t.taskStatus
}

func normalizedTaskToken(name string) string {
	return telemetry.NormalizeTelemetryToken(name)
}

func taskLogFields(taskToken string) []any {
	if taskToken == "" {
		return nil
	}
	return []any{"task", taskToken}
}

func taskRunLogFields(taskToken string, duration time.Duration, outcome string) []any {
	fields := append([]any{}, taskLogFields(taskToken)...)
	fields = append(fields, "duration", duration)
	if outcome != "" {
		fields = append(fields, "outcome", outcome)
	}
	return fields
}

func taskSpanAttributes(taskToken string) []attribute.KeyValue {
	if taskToken != "" {
		return []attribute.KeyValue{attribute.String("task.name", taskToken)}
	}
	return nil
}
