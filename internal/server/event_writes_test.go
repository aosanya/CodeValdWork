package server

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestWriteTracker_FiresAfterAllPathsConfirmed verifies the happy path: the
// gate releases once every expected git.file.written path has arrived.
func TestWriteTracker_FiresAfterAllPathsConfirmed(t *testing.T) {
	t.Parallel()
	tr := newWriteTracker(2 * time.Second)
	var fired int32
	done := make(chan struct{})

	tr.WaitForWrites("run-1", []string{"a.dart", "b.dart", "c.dart"}, func() {
		atomic.StoreInt32(&fired, 1)
		close(done)
	})

	tr.OnFileWritten("run-1", "a.dart")
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatalf("callback fired before all paths confirmed")
	}
	tr.OnFileWritten("run-1", "b.dart")
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatalf("callback fired before all paths confirmed")
	}
	tr.OnFileWritten("run-1", "c.dart")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("callback did not fire after all paths confirmed")
	}
}

// TestWriteTracker_FiresOnTimeout verifies the safety net: if confirmations
// never arrive the gate still releases (with a warning) so the pipeline does
// not stall on a lost git.file.written event.
func TestWriteTracker_FiresOnTimeout(t *testing.T) {
	t.Parallel()
	tr := newWriteTracker(80 * time.Millisecond)
	done := make(chan struct{})

	tr.WaitForWrites("run-2", []string{"never.dart"}, func() { close(done) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timeout fallback did not fire")
	}
}

// TestWriteTracker_PreArrivalBuffer covers the (rare) case in which a
// git.file.written event reaches the dispatcher before its matching
// task.completed event. The buffered confirmation must satisfy the wait
// without requiring a duplicate event.
func TestWriteTracker_PreArrivalBuffer(t *testing.T) {
	t.Parallel()
	tr := newWriteTracker(2 * time.Second)
	done := make(chan struct{})

	tr.OnFileWritten("run-3", "early.dart") // arrives first
	tr.WaitForWrites("run-3", []string{"early.dart"}, func() { close(done) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("pre-arrival buffer did not satisfy wait")
	}
}

// TestWriteTracker_OnlyFiresOnce ensures the callback runs at most once even
// when path confirmations and timeouts race, guarding against double-publish
// of the deferred work.todo.completed event.
func TestWriteTracker_OnlyFiresOnce(t *testing.T) {
	t.Parallel()
	tr := newWriteTracker(50 * time.Millisecond)
	var fired int32
	done := make(chan struct{}, 4)

	tr.WaitForWrites("run-4", []string{"x.dart"}, func() {
		atomic.AddInt32(&fired, 1)
		done <- struct{}{}
	})
	tr.OnFileWritten("run-4", "x.dart")
	time.Sleep(150 * time.Millisecond) // let any timer fire too

	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Fatalf("expected callback to fire exactly once, got %d", got)
	}
}
