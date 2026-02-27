package task

import (
	"context"
	"log/slog"
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
	counter    uint64
	taskStatus TaskStatus
}

func New(name string, exec func(ctx context.Context), contextValues map[ContextKey]any, interval time.Duration) Task {
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
		counter:    0,
		taskStatus: TaskStatusNotStarted,
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
				slog.Info("task running", "task", t.name, "counter", t.counter)
				t.counter++
				t.exec(t.context)
			}
		}
	}()
}

func (t *taskImpl) StartNoWait() {
	slog.Info("task running early", "task", t.name, "counter", t.counter)
	t.counter++
	t.exec(t.context)
	t.Start()
}

func (t *taskImpl) Stop() {
	t.cancelFunc()
	t.taskStatus = TaskStatusStopped
}

func (t *taskImpl) Status() TaskStatus {
	return t.taskStatus
}
