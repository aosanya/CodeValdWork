package server

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// writeGateTimeout caps how long an ai.task.completed payload may be held
// while waiting for its emitted git.file.write paths to come back as
// git.file.written events. When it fires the deferred callback runs anyway
// and a warning is logged so the pipeline cannot stall on a lost confirmation.
const writeGateTimeout = 30 * time.Second

// writeTracker correlates ai.task.completed payloads (which carry the list of
// emitted write paths) with the git.file.written events CodeValdGit publishes
// after each successful commit. It exists to close the BUG-09-020 race in
// which CodeValdWork could publish work.todo.completed before CodeValdGit had
// landed all the files for that todo run.
//
// The tracker is goroutine-safe and exposes a small surface:
//   - WaitForWrites registers an expected set of (runID, paths) and a callback
//     to run once every path has been confirmed (or the gate timeout fires).
//   - OnFileWritten records that one (runID, path) has been confirmed.
//
// A pre-arrival buffer covers the (uncommon) case where a git.file.written
// event reaches the dispatcher before its ai.task.completed event.
type writeTracker struct {
	timeout time.Duration

	mu    sync.Mutex
	waits map[string]*writeWait              // runID → active wait
	early map[string]map[string]struct{}     // runID → paths confirmed before WaitForWrites was called
}

// writeWait holds the state for a single in-flight gate.
type writeWait struct {
	remaining map[string]struct{} // paths still awaiting confirmation
	onDone    func()
	timer     *time.Timer
	fired     bool
}

// newWriteTracker constructs a tracker with the given gate timeout.
func newWriteTracker(timeout time.Duration) *writeTracker {
	return &writeTracker{
		timeout: timeout,
		waits:   make(map[string]*writeWait),
		early:   make(map[string]map[string]struct{}),
	}
}

// WaitForWrites registers an expected set of paths for runID. onDone runs in
// its own goroutine once every path has been confirmed by OnFileWritten, or
// after the gate timeout fires — whichever comes first. The callback is
// guaranteed to run exactly once.
//
// If every path has already arrived (via the pre-arrival buffer) onDone is
// dispatched synchronously to a goroutine before WaitForWrites returns.
func (t *writeTracker) WaitForWrites(runID string, paths []string, onDone func()) {
	if onDone == nil {
		return
	}
	if runID == "" || len(paths) == 0 {
		go onDone()
		return
	}

	t.mu.Lock()
	if existing, ok := t.waits[runID]; ok {
		// A second ai.task.completed for the same run is unexpected. Fire the
		// first wait's callback so it isn't stranded, then overwrite.
		log.Printf("codevaldwork: writeTracker: duplicate WaitForWrites for run=%s — releasing prior wait", runID)
		t.fireLocked(runID, existing, "duplicate")
	}

	remaining := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if p != "" {
			remaining[p] = struct{}{}
		}
	}
	if buf := t.early[runID]; buf != nil {
		for p := range buf {
			delete(remaining, p)
		}
		delete(t.early, runID)
	}
	if len(remaining) == 0 {
		t.mu.Unlock()
		go onDone()
		return
	}

	wait := &writeWait{remaining: remaining, onDone: onDone}
	// Assign the timer while holding t.mu so its eventual read inside
	// fireLocked (when the timer fires or OnFileWritten completes the gate)
	// is ordered after this write via the mutex's happens-before guarantee.
	wait.timer = time.AfterFunc(t.timeout, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		w, ok := t.waits[runID]
		if !ok || w != wait || w.fired {
			return
		}
		t.fireLocked(runID, w, "timeout")
	})
	t.waits[runID] = wait
	t.mu.Unlock()
}

// OnFileWritten records that the (runID, path) write has been confirmed and
// fires the gate callback when no more paths remain.
func (t *writeTracker) OnFileWritten(runID, path string) {
	if runID == "" || path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	wait, ok := t.waits[runID]
	if !ok {
		// Pre-arrival: WaitForWrites hasn't been called yet for this runID.
		buf := t.early[runID]
		if buf == nil {
			buf = make(map[string]struct{})
			t.early[runID] = buf
		}
		buf[path] = struct{}{}
		return
	}
	if wait.fired {
		return
	}
	delete(wait.remaining, path)
	if len(wait.remaining) == 0 {
		t.fireLocked(runID, wait, "all writes confirmed")
	}
}

// fireLocked releases the wait, stops its timer, and dispatches the callback.
// Caller must hold t.mu.
func (t *writeTracker) fireLocked(runID string, w *writeWait, reason string) {
	if w.fired {
		return
	}
	w.fired = true
	delete(t.waits, runID)
	if w.timer != nil {
		w.timer.Stop()
	}
	if reason == "timeout" {
		log.Printf("codevaldwork: writeTracker: gate timeout for run=%s remaining=%d — firing anyway", runID, len(w.remaining))
	} else {
		log.Printf("codevaldwork: writeTracker: %s for run=%s", reason, runID)
	}
	go w.onDone()
}

// gitFileWrittenPayload is the minimum we need from a git.file.written event.
// Mirrors the on-the-wire shape published by CodeValdGit; we copy the fields
// inline rather than importing CodeValdGit because CodeValdWork must not take
// a Go dependency on sibling services (CLAUDE.md cross-service rule).
type gitFileWrittenPayload struct {
	RunID string `json:"run_id"`
	Path  string `json:"path"`
}

// handleFileWritten consumes a git.file.written event and notifies the
// writeTracker so any matching ai.task.completed gate can advance.
func (d *TaskEventDispatcher) handleFileWritten(_ context.Context, payloadStr string) {
	if d.writes == nil {
		return
	}
	var p gitFileWrittenPayload
	if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
		log.Printf("codevaldwork: handleFileWritten: bad payload: %v", err)
		return
	}
	if p.RunID == "" {
		// Pre-BUG-09-020 git.file.written payloads (or non-AI writes) carry no
		// run_id; there's nothing to correlate against. Skip silently.
		return
	}
	d.writes.OnFileWritten(p.RunID, p.Path)
}
