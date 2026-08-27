package task

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCronNextUsesStrictUTCOccurrences(t *testing.T) {
	oslo, err := time.LoadLocation("Europe/Oslo")
	require.NoError(t, err)
	schedule := MustCron("*/15 * * * *")

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "between boundaries",
			now:  time.Date(2026, time.August, 26, 10, 7, 30, 0, time.UTC),
			want: time.Date(2026, time.August, 26, 10, 15, 0, 0, time.UTC),
		},
		{
			name: "exact boundary selects following occurrence",
			now:  time.Date(2026, time.August, 26, 10, 15, 0, 0, time.UTC),
			want: time.Date(2026, time.August, 26, 10, 30, 0, 0, time.UTC),
		},
		{
			name: "non UTC input is evaluated by UTC clock",
			now:  time.Date(2026, time.August, 26, 12, 7, 30, 0, oslo),
			want: time.Date(2026, time.August, 26, 10, 15, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, schedule.Next(tt.now))
		})
	}
}

func TestCronNextSupportsOffsetMinuteSteps(t *testing.T) {
	schedule := MustCron("10/20 * * * *")

	tests := []struct {
		now  time.Time
		want time.Time
	}{
		{time.Date(2026, 8, 26, 10, 1, 0, 0, time.UTC), time.Date(2026, 8, 26, 10, 10, 0, 0, time.UTC)},
		{time.Date(2026, 8, 26, 10, 10, 0, 0, time.UTC), time.Date(2026, 8, 26, 10, 30, 0, 0, time.UTC)},
		{time.Date(2026, 8, 26, 10, 31, 0, 0, time.UTC), time.Date(2026, 8, 26, 10, 50, 0, 0, time.UTC)},
		{time.Date(2026, 8, 26, 10, 51, 0, 0, time.UTC), time.Date(2026, 8, 26, 11, 10, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, schedule.Next(tt.now))
	}
}

func TestCronNextSupportsThreeHourSchedules(t *testing.T) {
	schedule := MustCron("0 */3 * * *")

	assert.Equal(t,
		time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC),
		schedule.Next(time.Date(2026, 8, 26, 2, 59, 0, 0, time.UTC)),
	)
	assert.Equal(t,
		time.Date(2026, 8, 26, 6, 0, 0, 0, time.UTC),
		schedule.Next(time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)),
	)
}

func TestParseCronRejectsInvalidOrNonFiveFieldExpressions(t *testing.T) {
	for _, expression := range []string{"", "*/15 * * *", "0 */3 * * * *", "60 * * * *", "not cron"} {
		_, err := ParseCron(expression)
		assert.Error(t, err, expression)
	}
}

func TestMustCronPanicsForInvalidConstant(t *testing.T) {
	assert.Panics(t, func() { MustCron("not cron") })
	assert.NotPanics(t, func() { MustCron("*/15 * * * *") })
}

func TestNewScheduledPanicsForNilSchedule(t *testing.T) {
	assert.PanicsWithValue(t, "task: schedule cannot be nil", func() {
		NewScheduled("nil-schedule", func(context.Context) {}, nil, nil, true)
	})
}

func TestScheduledTaskStartsImmediatelyRecalculatesAndStops(t *testing.T) {
	clock := newFakeTaskClock(time.Date(2026, 8, 26, 10, 7, 0, 0, time.UTC))
	var executions atomic.Int32
	exec := func(context.Context) {
		if executions.Add(1) == 2 {
			clock.Set(time.Date(2026, 8, 26, 10, 16, 0, 0, time.UTC))
		}
	}

	scheduled := newScheduled(
		"scheduled-test",
		exec,
		nil,
		MustCron("*/15 * * * *"),
		true,
		clock,
	)
	scheduled.StartNoWait()
	assert.Equal(t, int32(1), executions.Load())
	assert.Equal(t, TaskStatusRunning, scheduled.Status())

	first := clock.NextTimer(t)
	assert.Equal(t, 8*time.Minute, first.duration)
	clock.Set(time.Date(2026, 8, 26, 10, 15, 0, 0, time.UTC))
	assert.True(t, first.Fire())
	require.Eventually(t, func() bool { return executions.Load() == 2 }, time.Second, time.Millisecond)

	second := clock.NextTimer(t)
	assert.Equal(t, 14*time.Minute, second.duration)
	scheduled.Stop()
	assert.Equal(t, TaskStatusStopped, scheduled.Status())
	require.Eventually(t, second.Stopped, time.Second, time.Millisecond)
	assert.False(t, second.Fire())
	assert.Equal(t, int32(2), executions.Load())
}

func TestRunExecSkipsCanceledTask(t *testing.T) {
	var executions atomic.Int32
	task := New("canceled-test", func(context.Context) {
		executions.Add(1)
	}, nil, time.Second, true).(*taskImpl)

	task.cancelFunc()
	task.runExec()

	assert.Zero(t, executions.Load())
}

type fakeTaskClock struct {
	mu     sync.Mutex
	now    time.Time
	timers chan *fakeTaskTimer
}

func newFakeTaskClock(now time.Time) *fakeTaskClock {
	return &fakeTaskClock{now: now, timers: make(chan *fakeTaskTimer, 4)}
}

func (c *fakeTaskClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeTaskClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *fakeTaskClock) NewTimer(duration time.Duration) taskTimer {
	timer := &fakeTaskTimer{duration: duration, ch: make(chan time.Time, 1)}
	c.timers <- timer
	return timer
}

func (c *fakeTaskClock) NextTimer(t *testing.T) *fakeTaskTimer {
	t.Helper()
	select {
	case timer := <-c.timers:
		return timer
	case <-time.After(time.Second):
		t.Fatal("scheduled task did not create its next timer")
		return nil
	}
}

type fakeTaskTimer struct {
	mu       sync.Mutex
	duration time.Duration
	ch       chan time.Time
	stopped  bool
}

func (t *fakeTaskTimer) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTaskTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

func (t *fakeTaskTimer) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

func (t *fakeTaskTimer) Fire() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return false
	}
	t.ch <- time.Time{}
	return true
}

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
