// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: a run, and everything it does, is unexported. Nothing constructs one
// until a call into an artifact does, which is a later phase in another
// package, so there is no surface a test could reach this through.
package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

const (
	FIRST_NAME   = "first"
	SECOND_NAME  = "second"
	THREAD_NAME  = "spine"
	RACING_COUNT = 64
)

// _Started returns a run as a call into an artifact would leave one, including
// a context it can cancel itself - which is what a failed assertion uses.
//
// Revisions:
//   - 2026-09-19 21:57: initial creation
func _Started(ctx context.Context) *_Run {
	inner, stop := context.WithCancel(ctx)

	return &_Run{
		ctx:  inner,
		stop: stop,
		next: FIRST_SPAWN,
	}
}

// _Tracked returns a handle as spawn would hand one to _Track.
//
// Revisions:
//   - 2026-09-19 21:57: initial creation
func _Tracked(name string, stop context.CancelFunc) *Handle {
	return &Handle{
		name: name,
		done: make(chan struct{}),
		stop: stop,
	}
}

// TestTrack_NumbersFromAfterTheSpine proves spawns take 1 upwards in the order
// they were started, leaving 0 for the entry point - which is the numbering
// workflow.proto records.
//
// Revisions:
//   - 2026-09-19 21:58: initial creation
func TestTrack_NumbersFromAfterTheSpine(t *testing.T) {
	run := _Started(t.Context())

	first := _Tracked(FIRST_NAME, func() {})
	second := _Tracked(SECOND_NAME, func() {})

	run._Track(first)
	run._Track(second)

	if first.Thread() != FIRST_SPAWN {
		t.Fatalf("%s is thread %d, want %d", first.Name(), first.Thread(), FIRST_SPAWN)
	}

	if second.Thread() != FIRST_SPAWN+1 {
		t.Fatalf("%s is thread %d, want %d", second.Name(), second.Thread(), FIRST_SPAWN+1)
	}

	if SPINE >= FIRST_SPAWN {
		t.Fatalf("the spine is %d and the first spawn %d, which collide", SPINE, FIRST_SPAWN)
	}
}

// TestTrack_GivesRacingSpawnsDistinctNumbers proves numbering happens under the
// same lock that records the handle. Without it two spawns can read one number,
// and a graph would show two threads as one.
//
// This only means anything under -race, and under it the detector also proves
// the live list is not corrupted.
//
// Revisions:
//   - 2026-09-19 21:59: initial creation
func TestTrack_GivesRacingSpawnsDistinctNumbers(t *testing.T) {
	run := _Started(t.Context())

	var group sync.WaitGroup

	handles := make([]*Handle, RACING_COUNT)

	for index := range RACING_COUNT {
		handles[index] = _Tracked(FIRST_NAME, func() {})

		group.Add(1)

		go func() {
			defer group.Done()

			run._Track(handles[index])
		}()
	}

	group.Wait()

	seen := map[int32]bool{}

	for _, handle := range handles {
		if seen[handle.Thread()] {
			t.Fatalf("thread %d was given to two handles", handle.Thread())
		}

		seen[handle.Thread()] = true
	}

	if len(seen) != RACING_COUNT {
		t.Fatalf("%d handles got %d numbers", RACING_COUNT, len(seen))
	}
}

// TestAbandon_CancelsEverythingStillRunning proves a handle nobody joined is
// stopped when the run ends, rather than waited for.
//
// Revisions:
//   - 2026-09-19 22:00: initial creation
func TestAbandon_CancelsEverythingStillRunning(t *testing.T) {
	run := _Started(t.Context())

	stopped := make([]bool, 2)

	for index := range stopped {
		run._Track(_Tracked(FIRST_NAME, func() {
			stopped[index] = true
		}))
	}

	run._Abandon()

	for index, done := range stopped {
		if !done {
			t.Fatalf("handle %d was left running", index)
		}
	}
}

// TestOf_FindsTheRunOnAThread proves a builtin can reach its run, since the
// interpreter hands a builtin nothing else.
//
// Revisions:
//   - 2026-09-19 22:01: initial creation
func TestOf_FindsTheRunOnAThread(t *testing.T) {
	run := _Started(t.Context())

	thread := &starlark.Thread{Name: THREAD_NAME}
	thread.SetLocal(RUN_KEY, run)
	thread.SetLocal(THREAD_KEY, int32(SPINE))

	found, err := _Of(thread)
	if err != nil {
		t.Fatalf("_Of: %v", err)
	}

	if found != run {
		t.Fatal("_Of returned a different run")
	}

	if thread.Local(THREAD_KEY) != int32(SPINE) {
		t.Fatalf("thread number is %v, want %d", thread.Local(THREAD_KEY), SPINE)
	}
}

// TestOf_RefusesAThreadWithNoRun proves a builtin called on a thread nothing
// set up is told so, rather than reaching a nil run.
//
// Revisions:
//   - 2026-09-19 22:01: initial creation
func TestOf_RefusesAThreadWithNoRun(t *testing.T) {
	_, err := _Of(&starlark.Thread{Name: THREAD_NAME})
	if !errors.Is(err, ErrNoRun) {
		t.Fatalf("got %v, want ErrNoRun", err)
	}
}

// TestRun_CarriesWhatLaterPhasesUse proves the two fields no code in this phase
// touches are present and usable: the context spawn derives a child from, and
// the wait group a call waits on.
//
// As in handle_internal_test.go, this test is the only thing referring to them
// until spawn arrives. They are not speculative - the frozen phase declares
// both - but a reader should know why they are here.
//
// Revisions:
//   - 2026-09-19 22:02: initial creation
func TestRun_CarriesWhatLaterPhasesUse(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())

	run := _Started(ctx)

	if run.ctx == nil {
		t.Fatal("a run carries no context for spawn to derive from")
	}

	run.group.Add(1)

	go run.group.Done()

	run.group.Wait()

	stop()

	if run.ctx.Err() == nil {
		t.Fatal("cancelling the run's context did not reach it")
	}
}
