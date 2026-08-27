package task

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
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

type Schedule interface {
	Next(time.Time) time.Time
}

type taskTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type taskClock interface {
	Now() time.Time
	NewTimer(time.Duration) taskTimer
}

type realTaskClock struct{}

func (realTaskClock) Now() time.Time {
	return time.Now().UTC()
}

func (realTaskClock) NewTimer(duration time.Duration) taskTimer {
	return &realTaskTimer{timer: time.NewTimer(duration)}
}

type realTaskTimer struct {
	timer *time.Timer
}

func (t *realTaskTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t *realTaskTimer) Stop() bool {
	return t.timer.Stop()
}

type utcSchedule struct {
	schedule cron.Schedule
}

func (s utcSchedule) Next(now time.Time) time.Time {
	return s.schedule.Next(now.UTC()).UTC()
}

var fiveFieldCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func ParseCron(expression string) (Schedule, error) {
	schedule, err := fiveFieldCronParser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("parse five-field cron expression %q: %w", expression, err)
	}
	return utcSchedule{schedule: schedule}, nil
}

func MustCron(expression string) Schedule {
	schedule, err := ParseCron(expression)
	if err != nil {
		panic(err)
	}
	return schedule
}

var _ Task = (*taskImpl)(nil)

type taskImpl struct {
	name       string
	exec       func(ctx context.Context)
	context    context.Context
	cancelFunc context.CancelFunc
	interval   time.Duration
	schedule   Schedule
	clock      taskClock
	statusMu   sync.RWMutex
	taskStatus TaskStatus
	silent     bool
}

func New(name string, exec func(ctx context.Context), contextValues map[ContextKey]any, interval time.Duration, silent bool) Task {
	return newTask(name, exec, contextValues, interval, nil, silent, nil)
}

func NewScheduled(
	name string,
	exec func(context.Context),
	contextValues map[ContextKey]any,
	schedule Schedule,
	silent bool,
) Task {
	return newScheduled(name, exec, contextValues, schedule, silent, realTaskClock{})
}

func newScheduled(
	name string,
	exec func(context.Context),
	contextValues map[ContextKey]any,
	schedule Schedule,
	silent bool,
	clock taskClock,
) Task {
	if schedule == nil {
		panic("task: schedule cannot be nil")
	}
	return newTask(name, exec, contextValues, 0, schedule, silent, clock)
}

func newTask(
	name string,
	exec func(context.Context),
	contextValues map[ContextKey]any,
	interval time.Duration,
	schedule Schedule,
	silent bool,
	clock taskClock,
) Task {
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
		schedule:   schedule,
		clock:      clock,
		taskStatus: TaskStatusNotStarted,
		silent:     silent,
	}
}

func (t *taskImpl) Start() {
	t.setStatus(TaskStatusRunning)
	if t.schedule != nil {
		go t.runScheduled()
		return
	}

	ticker := time.NewTicker(t.interval)

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

func (t *taskImpl) runScheduled() {
	for {
		now := t.clock.Now().UTC()
		next := t.schedule.Next(now)
		if !next.After(now) {
			slog.Error("task schedule returned a non-future occurrence", "task", t.name, "now", now, "next", next)
			t.setStatus(TaskStatusStopped)
			return
		}

		timer := t.clock.NewTimer(next.Sub(now))
		select {
		case <-t.context.Done():
			timer.Stop()
			slog.Info("task stopped", "task", t.name)
			return
		case <-timer.C():
			timer.Stop()
			if !t.silent {
				slog.Info("task running", "task", t.name)
			}
			t.runExec()
		}
	}
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
	if t.context.Err() != nil {
		return
	}
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
	t.setStatus(TaskStatusStopped)
}

func (t *taskImpl) Status() TaskStatus {
	t.statusMu.RLock()
	defer t.statusMu.RUnlock()
	return t.taskStatus
}

func (t *taskImpl) setStatus(status TaskStatus) {
	t.statusMu.Lock()
	defer t.statusMu.Unlock()
	t.taskStatus = status
}
