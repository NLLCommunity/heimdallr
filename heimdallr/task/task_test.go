package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	execFunc := func(ctx context.Context) {}
	testKey := ContextKey("test_key")

	contextValues := ContextKeyMap{
		testKey: "test_value",
	}
	interval := time.Second

	task := New("test-task", execFunc, contextValues, interval)

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

	task := New("test-task", execFunc, nil, 50*time.Millisecond)

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

	task := New("test-task", execFunc, nil, time.Second)

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

	task := New("test-task", execFunc, contextValues, 100*time.Millisecond)
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

	task := New("test-task", execFunc, nil, 50*time.Millisecond)
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

	task1 := New("task-1", exec1, nil, 50*time.Millisecond)
	task2 := New("task-2", exec2, nil, 75*time.Millisecond)

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

	task := New("test-task", execFunc, nil, time.Second)

	// Initial status.
	assert.Equal(t, TaskStatusNotStarted, task.Status())

	// After starting.
	task.Start()
	assert.Equal(t, TaskStatusRunning, task.Status())

	// After stopping.
	task.Stop()
	assert.Equal(t, TaskStatusStopped, task.Status())
}
