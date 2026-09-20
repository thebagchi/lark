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
		ctx:     inner,
		stop:    stop,
		ordinal: make(map[string]int32),
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

// TestTrack_NamesAThreadAfterItsParent proves an id says whose child it is, and
// that the spine contributes no prefix.
//
// Revisions:
//   - 2026-09-19 21:58: initial creation, as TestTrack_NumbersFromAfterTheSpine
//   - 2026-09-21 00:59: an id names its parent rather than counting from after
//     the spine
func TestTrack_NamesAThreadAfterItsParent(t *testing.T) {
	run := _Started(t.Context())

	first := _Tracked(FIRST_NAME, func() {})
	second := _Tracked(SECOND_NAME, func() {})
	deep := _Tracked(FIRST_NAME, func() {})

	run._Track(first, SPINE)
	run._Track(second, SPINE)
	run._Track(deep, first.Thread())

	if first.Thread() != "thread_1" {
		t.Fatalf("%s is %s, want thread_1", first.Name(), first.Thread())
	}

	if second.Thread() != "thread_2" {
		t.Fatalf("%s is %s, want thread_2", second.Name(), second.Thread())
	}

	if deep.Thread() != "thread_1_1" {
		t.Fatalf("a child of %s is %s, want thread_1_1", first.Thread(), deep.Thread())
	}
}

// TestTrack_CountsPerParentRatherThanPerRun is the defect a shared counter has.
//
// One counter for the whole run numbers in the order spawns happen, so a
// sibling started after a nephew takes the higher number and the ids stop
// describing the tree. Counted per parent, the second child of the spine is
// thread_2 whatever else started first.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func TestTrack_CountsPerParentRatherThanPerRun(t *testing.T) {
	run := _Started(t.Context())

	first := _Tracked(FIRST_NAME, func() {})
	nephew := _Tracked(FIRST_NAME, func() {})
	second := _Tracked(SECOND_NAME, func() {})

	run._Track(first, SPINE)
	run._Track(nephew, first.Thread())
	run._Track(second, SPINE)

	if second.Thread() != "thread_2" {
		t.Fatalf("a nephew started first made the second child %s", second.Thread())
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

			run._Track(handles[index], SPINE)
		}()
	}

	group.Wait()

	seen := map[string]bool{}

	for _, handle := range handles {
		if seen[handle.Thread()] {
			t.Fatalf("thread %s was given to two handles", handle.Thread())
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
		}), SPINE)
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
	thread.SetLocal(THREAD_KEY, SPINE)

	found, err := _Of(thread)
	if err != nil {
		t.Fatalf("_Of: %v", err)
	}

	if found != run {
		t.Fatal("_Of returned a different run")
	}

	if thread.Local(THREAD_KEY) != SPINE {
		t.Fatalf("thread id is %v, want %s", thread.Local(THREAD_KEY), SPINE)
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
