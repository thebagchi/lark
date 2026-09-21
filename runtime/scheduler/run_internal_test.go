// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: a run, and everything it does, is unexported. Nothing constructs one
// until a call into an artifact does, which is another package, so there is no
// surface a test could reach this through.
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
//   - 2026-09-21 08:09: builds the maps a run now holds
func _Started(ctx context.Context) *_Run {
	inner, stop := context.WithCancel(ctx)

	return &_Run{
		ctx:     inner,
		stop:    stop,
		ordinal: make(map[string]int32),
		shared:  make(map[string]any),
		locks:   make(map[string]chan struct{}),
	}
}

// _Thread returns an interpreter thread carrying run's spine evaluation, as a
// call into an artifact would leave one.
//
// Revisions:
//   - 2026-09-19 22:03: initial creation
//   - 2026-09-21 08:09: leaves the one local
func _Thread(run *_Run) *starlark.Thread {
	thread := &starlark.Thread{Name: THREAD_NAME}
	thread.SetLocal(LOCALS_KEY, &_Locals{run: run, thread: SPINE, ctx: run.ctx})

	return thread
}

// TestNumber_NamesAThreadAfterItsParent proves an id says whose child it is,
// and that the spine contributes no prefix.
//
// Revisions:
//   - 2026-09-19 21:58: initial creation, as TestTrack_NumbersFromAfterTheSpine
//   - 2026-09-21 00:59: an id names its parent rather than counting from after
//     the spine
//   - 2026-09-21 08:09: numbering is its own call, since nothing tracks handles
func TestNumber_NamesAThreadAfterItsParent(t *testing.T) {
	run := _Started(t.Context())

	first := run._Number(SPINE)
	second := run._Number(SPINE)
	deep := run._Number(first)

	if first != "thread_1" || second != "thread_2" {
		t.Fatalf("the spine's children are %s and %s, want thread_1 and thread_2", first, second)
	}

	if deep != "thread_1_1" {
		t.Fatalf("a child of %s is %s, want thread_1_1", first, deep)
	}
}

// TestNumber_CountsPerParentRatherThanPerRun is the defect a shared counter
// has: a sibling started after a nephew would take the higher number and the
// ids would stop describing the tree.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func TestNumber_CountsPerParentRatherThanPerRun(t *testing.T) {
	run := _Started(t.Context())

	first := run._Number(SPINE)
	run._Number(first)

	if second := run._Number(SPINE); second != "thread_2" {
		t.Fatalf("a nephew started first made the second child %s", second)
	}
}

// TestNumber_GivesRacingSpawnsDistinctIds proves numbering happens under a
// lock. Without it two spawns can read one number, and a graph would show two
// threads as one.
//
// Revisions:
//   - 2026-09-19 21:59: initial creation
func TestNumber_GivesRacingSpawnsDistinctIds(t *testing.T) {
	run := _Started(t.Context())

	var (
		group sync.WaitGroup
		guard sync.Mutex
	)

	seen := map[string]bool{}

	for range RACING_COUNT {
		group.Add(1)

		go func() {
			defer group.Done()

			id := run._Number(SPINE)

			guard.Lock()
			defer guard.Unlock()

			seen[id] = true
		}()
	}

	group.Wait()

	if len(seen) != RACING_COUNT {
		t.Fatalf("%d spawns got %d ids", RACING_COUNT, len(seen))
	}
}

// TestOf_FindsTheRunOnAThread proves a builtin can reach its evaluation, since
// the interpreter hands a builtin nothing else.
//
// Revisions:
//   - 2026-09-19 22:01: initial creation
func TestOf_FindsTheRunOnAThread(t *testing.T) {
	run := _Started(t.Context())

	found, err := _Of(_Thread(run))
	if err != nil {
		t.Fatalf("_Of: %v", err)
	}

	if found.run != run || found.thread != SPINE {
		t.Fatalf("_Of returned %+v", found)
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

// TestFail_EndsTheRunUnlessSomethingIsCatching is the one rule Fail exists
// for: the same failure ends the run from a plain evaluation and ends nothing
// from one that is catching.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func TestFail_EndsTheRunUnlessSomethingIsCatching(t *testing.T) {
	caught := _Started(t.Context())

	catching := _Thread(caught)
	catching.SetLocal(LOCALS_KEY, &_Locals{run: caught, thread: SPINE, ctx: caught.ctx, catching: 1})

	if err := Fail(catching, ErrNested); !errors.Is(err, ErrNested) {
		t.Fatalf("want the cause back, got %v", err)
	}

	if caught._Outcome() != nil || caught.ctx.Err() != nil {
		t.Fatal("a caught failure ended the run")
	}

	plain := _Started(t.Context())

	if err := Fail(_Thread(plain), ErrNested); !errors.Is(err, ErrNested) {
		t.Fatalf("want the cause back, got %v", err)
	}

	if !errors.Is(plain._Outcome(), ErrNested) || plain.ctx.Err() == nil {
		t.Fatalf("an uncaught failure left the run with %v", plain._Outcome())
	}
}
