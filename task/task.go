package task

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
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

	go func() {
		for {
			select {
			case <-t.context.Done():
				slog.Info("task stopped", "task", t.name)
				ticker.Stop()
				return
			case <-ticker.C:
				if !t.silent {
					slog.Info("task running", "task", t.name)
				}
				t.runExec()
			}
		}
	}()
}

func (t *taskImpl) StartNoWait() {
	if !t.silent {
		slog.Info("task running early", "task", t.name)
	}
	t.runExec()
	t.Start()
}

// runExec runs the task's exec function, recovering any panic so that a bug in a
// periodic task logs and skips the run instead of crashing the whole process.
func (t *taskImpl) runExec() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(
				"recovered from panic in scheduled task",
				"task", t.name,
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
	}()
	t.exec(t.context)
}

func (t *taskImpl) Stop() {
	t.cancelFunc()
	t.taskStatus = TaskStatusStopped
}

func (t *taskImpl) Status() TaskStatus {
	return t.taskStatus
}
