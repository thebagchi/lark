package tests

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"weak"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

const (
	// SHARED_FIRST and SHARED_SECOND are two keys of one run.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHARED_FIRST  = "shared.first"
	SHARED_SECOND = "shared.second"

	// SHARED_THREADS is how many threads race for the first use of a key.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHARED_THREADS = 64

	// SHARED_COLLECTIONS is how many collections the release test forces
	// before it asks a weak pointer whether its object is gone.
	//
	// Revisions:
	//   - 2026-11-02 21:22: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 22:45: moved to the external package (C65c)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHARED_COLLECTIONS = 4
)

// TestSharedIn_ConcurrentFirstUseGivesOneValue proves threads that race for
// the first use of a key all see one value, and that build runs once.
//
// Run with -race: the check is also that the detector stays clean while the
// threads read and one of them writes.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSharedIn_ConcurrentFirstUseGivesOneValue(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		builds atomic.Int32
		gate   = make(chan struct{})
		group  sync.WaitGroup
		seen   = make(
			[]*int,
			SHARED_THREADS,
		)
	)

	for index := range SHARED_THREADS {
		group.Go(func() {
			evaluation, err := run.EvaluationOf(thread)
			if err != nil {
				t.Error(err)

				return
			}

			<-gate

			seen[index], err = run.SharedIn(
				evaluation,
				SHARED_FIRST,
				func() *int {
					builds.Add(1)

					return new(int)
				},
			)
			if err != nil {
				t.Error(err)
			}
		})
	}

	close(gate)
	group.Wait()

	if builds.Load() != 1 {
		t.Errorf(
			"build ran %d times, want 1",
			builds.Load(),
		)
	}

	for index, got := range seen {
		if got == nil || got != seen[0] {
			t.Fatalf(
				"thread %d saw %p, want %p",
				index,
				got,
				seen[0],
			)
		}
	}
}

// TestSharedIn_ReadersRaceAWriterOfAnotherKey proves a thread that reads a
// key held already is not disturbed by threads that add other keys.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSharedIn_ReadersRaceAWriterOfAnotherKey(t *testing.T) {
	var (
		thread, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		first, err  = run.EvaluationOf(thread)
		held        *int
		group       sync.WaitGroup
		keys        = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		made, wrong atomic.Int32
	)
	if err != nil {
		t.Fatal(err)
	}

	held, err = run.SharedIn(
		first,
		SHARED_FIRST,
		func() *int {
			return new(int)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range keys {
		group.Go(func() {
			_, err := run.SharedIn(
				first,
				key,
				func() *int {
					made.Add(1)

					return new(int)
				},
			)
			if err != nil {
				t.Error(err)
			}
		})

		group.Go(func() {
			got, err := run.SharedIn(
				first,
				SHARED_FIRST,
				func() *int {
					t.Error("a second build of a held key")

					return nil
				},
			)
			if err != nil || got != held {
				wrong.Add(1)
			}
		})
	}

	group.Wait()

	if made.Load() != int32(len(keys)) || wrong.Load() != 0 {
		t.Fatalf(
			"built %d keys and %d readers saw another value, want %d and 0",
			made.Load(),
			wrong.Load(),
			len(keys),
		)
	}
}

// TestSharedIn_RunsStayIsolated proves two runs hold two values for one key,
// and one run holds two values for two keys.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSharedIn_RunsStayIsolated(t *testing.T) {
	var (
		threadOne, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		threadTwo, _ = _Open(
			t.Context(),
			t,
			nil,
		)
		one, errOne = run.EvaluationOf(threadOne)
		two, errTwo = run.EvaluationOf(threadTwo)
	)
	if errOne != nil || errTwo != nil {
		t.Fatalf(
			"evaluations: %v and %v",
			errOne,
			errTwo,
		)
	}

	var (
		build = func() *int {
			return new(int)
		}
		a, _ = run.SharedIn(
			one,
			SHARED_FIRST,
			build,
		)
		b, _ = run.SharedIn(
			two,
			SHARED_FIRST,
			build,
		)
		c, _ = run.SharedIn(
			one,
			SHARED_SECOND,
			build,
		)
		a2, _ = run.SharedIn(
			one,
			SHARED_FIRST,
			build,
		)
	)

	if a == b {
		t.Error("two runs share one value for one key")
	}

	if a == c {
		t.Error("two keys of one run share one value")
	}

	if a != a2 {
		t.Error("one key of one run gave two values")
	}
}

// _Released builds a value in a run, ends the run, and returns a weak pointer
// to the value. Nothing in this function names the run or value afterwards.
// It does not use _Open: the cleanup _Open registers would keep the run, and so
// the value, reachable until the test ends.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Released(t *testing.T) weak.Pointer[int] {
	t.Helper()

	var (
		thread = &starlark.Thread{}
		finish = run.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			nil,
		)
		evaluation, err = run.EvaluationOf(thread)
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.SharedIn(
		evaluation,
		SHARED_FIRST,
		func() *int {
			return new(int)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := finish(nil); err != nil {
		t.Fatal(err)
	}

	return weak.Make(value)
}

// TestSharedIn_ValueIsReleasedWithItsRun proves a value a run shared is
// collectable once the run and its threads are unreachable, as C33 checked
// for a handle.
//
// Check by storing the value in a package variable: the test must then fail.
//
// Revisions:
//   - 2026-11-02 21:01: initial creation, for change C39a
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSharedIn_ValueIsReleasedWithItsRun(t *testing.T) {
	value := _Released(t)

	for range SHARED_COLLECTIONS {
		runtime.GC()
	}

	if value.Value() != nil {
		t.Error("a value of a finished run is still reachable")
	}
}
