package artifact_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/script"
)

const (
	// KEEPS is an entry that returns what LOADABLE binds to KEPT, so two runs
	// can be asked whether they were handed one value or two.
	KEEPS = `
load("lib.star", "KEPT")

def main():
    return KEPT
`

	// OTHER is the module a case of LOADABLE may load in turn.
	OTHER = "other.star"

	// PURE is a module whose top level calls nothing.
	PURE = `
KEPT = [1]
`

	// PRINTS is a module whose top level prints TOP.
	PRINTS = `
print("top")

KEPT = []
`
	TOP = "top"

	// DIVIDES is a module whose top level calls nothing and fails.
	DIVIDES = `
KEPT = 1 // 0
`

	// FOREVER is a module whose top level calls nothing and never ends.
	FOREVER = `
while True:
    pass

KEPT = []
`

	// RUNS_AT_ONCE is how many first runs of one artifact start together.
	RUNS_AT_ONCE = 16
)

// _Linked compiles sources[MAIN], loading from sources, or ends the test.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _Linked(
	t *testing.T,
	sources _Sources,
) *artifact.Artifact {
	t.Helper()

	built, err := artifact.Compile(&script.Source{
		Entry:  MAIN,
		Text:   []byte(sources[MAIN]),
		Loader: sources,
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	return built
}

// _Ran runs built and ends the test if the run fails.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _Ran(
	t *testing.T,
	built *artifact.Artifact,
	opts ...artifact.RunOption,
) starlark.Value {
	t.Helper()

	value, err := artifact.Run(
		t.Context(),
		built,
		opts...,
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	return value
}

// TestRun_SharesWhatOnlyAPureModuleBuilt checks which modules a run builds
// again. A module whose top level calls nothing, and loads only modules like
// it, is built by the first run, and every later run gets the same value. Any
// other module is built by every run, as before.
//
// A call inside a def or a lambda body runs only when the function does, so
// it leaves the module pure. A call in a default runs at the top level, so it
// does not.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_SharesWhatOnlyAPureModuleBuilt(t *testing.T) {
	cases := []struct {
		name   string
		lib    string
		other  string
		shared bool
	}{
		{
			name:   "literals",
			lib:    `KEPT = [1, {"a": (2, 3)}]`,
			shared: true,
		},
		{
			name: "a def whose body calls",
			lib: `
def f():
    return len("ab")

KEPT = [f]
`,
			shared: true,
		},
		{
			name: "a lambda whose body calls",
			lib: `
g = lambda: len("ab")

KEPT = [g]
`,
			shared: true,
		},
		{
			name:   "a comprehension",
			lib:    `KEPT = [x * 2 for x in [1, 2] if x > 0]`,
			shared: true,
		},
		{
			name: "top-level control",
			lib: `
if 1 > 0:
    KEPT = [1]
`,
			shared: true,
		},
		{
			name: "a load of a pure module",
			lib: `
load("other.star", "N")

KEPT = [N]
`,
			other:  `N = 1`,
			shared: true,
		},
		{
			name:   "a call of len",
			lib:    `KEPT = [len("ab")]`,
			shared: false,
		},
		{
			name:   "a call of print",
			lib:    PRINTS,
			shared: false,
		},
		{
			name: "a def whose default calls",
			lib: `
def f(n = len("ab")):
    return n

KEPT = [f]
`,
			shared: false,
		},
		{
			name: "a lambda whose default calls",
			lib: `
g = lambda n = len("ab"): n

KEPT = [g]
`,
			shared: false,
		},
		{
			name:   "a comprehension that calls",
			lib:    `KEPT = [len(x) for x in ["a"]]`,
			shared: false,
		},
		{
			name: "a load of a module that calls",
			lib: `
load("other.star", "N")

KEPT = [N]
`,
			other:  `N = len("ab")`,
			shared: false,
		},
	}

	for _, each := range cases {
		t.Run(
			each.name,
			func(t *testing.T) {
				built := _Linked(
					t,
					_Sources{
						MAIN:     KEEPS,
						LOADABLE: each.lib,
						OTHER:    each.other,
					},
				)

				var (
					quiet = artifact.WithLog(io.Discard)
					first = _Ran(
						t,
						built,
						quiet,
					)
					second = _Ran(
						t,
						built,
						quiet,
					)
				)

				if (first == second) != each.shared {
					t.Fatalf(
						"two runs shared the value: %t, want %t",
						first == second,
						each.shared,
					)
				}
			},
		)
	}
}

// TestRun_ATopLevelPrintIsLoggedByEveryRun checks that a module whose top
// level prints is built by each run, so each run logs the line.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_ATopLevelPrintIsLoggedByEveryRun(t *testing.T) {
	var (
		built = _Linked(
			t,
			_Sources{
				MAIN:     KEEPS,
				LOADABLE: PRINTS,
			},
		)
		into   = new(_Spine)
		logger = slog.New(slog.NewJSONHandler(
			into,
			nil,
		))
	)

	for range RUNS_OF_ONE {
		_Ran(
			t,
			built,
			artifact.WithLogger(logger),
		)
	}

	printed := 0

	for _, line := range into.lines {
		if strings.HasSuffix(
			line,
			" "+TOP,
		) {
			printed++
		}
	}

	if printed != RUNS_OF_ONE {
		t.Fatalf(
			"the top level printed %d times in %d runs: %q",
			printed,
			RUNS_OF_ONE,
			into.lines,
		)
	}
}

// TestRun_APureModuleThatFailsFailsEveryRun checks that a pure module whose
// top level fails is a failure of each run, not of the compile.
//
// The first run keeps the failure, and every later run gets that same error.
// A top level that calls nothing has no input, so a second try fails the same
// way.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_APureModuleThatFailsFailsEveryRun(t *testing.T) {
	built := _Linked(
		t,
		_Sources{
			MAIN:     KEEPS,
			LOADABLE: DIVIDES,
		},
	)

	var failures []*starlark.EvalError

	for attempt := range RUNS_OF_ONE {
		_, err := artifact.Run(
			t.Context(),
			built,
		)

		var failure *starlark.EvalError
		if !errors.As(
			err,
			&failure,
		) {
			t.Fatalf(
				"run %d gave %v, want the module's own failure",
				attempt,
				err,
			)
		}

		failures = append(
			failures,
			failure,
		)
	}

	if failures[0] != failures[1] {
		t.Fatalf(
			"the second run gave %v, want the error the first run kept",
			failures[1],
		)
	}
}

// TestRun_AStoppedRunLeavesAPureModuleUnbuilt checks that a run stopped while
// it builds a pure module keeps nothing. The stop is the run's, not the
// module's, so the next run builds the module again and waits for its own
// deadline. A kept stop would end the next run at once.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_AStoppedRunLeavesAPureModuleUnbuilt(t *testing.T) {
	built := _Linked(
		t,
		_Sources{
			MAIN:     KEEPS,
			LOADABLE: FOREVER,
		},
	)

	_Stopped(
		t,
		built,
	)

	began := time.Now()

	_Stopped(
		t,
		built,
	)

	took := time.Since(began)
	if took < CANCEL_AFTER {
		t.Fatalf(
			"the next run stopped after %v, before its own deadline of %v",
			took,
			CANCEL_AFTER,
		)
	}
}

// TestRun_ARunWaitingOnAPureModuleCanBeStopped checks that a run which waits
// for another run to build a pure module still stops when its own context
// does.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_ARunWaitingOnAPureModuleCanBeStopped(t *testing.T) {
	var (
		built = _Linked(
			t,
			_Sources{
				MAIN:     KEEPS,
				LOADABLE: FOREVER,
			},
		)
		first = make(
			chan error,
			1,
		)
	)

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	go func() {
		_, err := artifact.Run(
			ctx,
			built,
		)
		first <- err
	}()

	time.Sleep(CANCEL_AFTER)

	_Stopped(
		t,
		built,
	)

	stop()

	select {
	case err := <-first:
		if !errors.Is(
			err,
			scheduler.ErrCancelled,
		) {
			t.Fatalf(
				"the first run gave %v, want ErrCancelled",
				err,
			)
		}
	case <-time.After(CANCEL_BUDGET):
		t.Fatal("the first run did not stop")
	}
}

// _Stopped runs built under a deadline and ends the test unless the run stops
// with ErrCancelled within CANCEL_BUDGET.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _Stopped(
	t *testing.T,
	built *artifact.Artifact,
) {
	t.Helper()

	ctx, stop := context.WithTimeout(
		t.Context(),
		CANCEL_AFTER,
	)
	defer stop()

	done := make(
		chan error,
		1,
	)

	go func() {
		_, err := artifact.Run(
			ctx,
			built,
		)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(
			err,
			scheduler.ErrCancelled,
		) {
			t.Fatalf(
				"the waiting run gave %v, want ErrCancelled",
				err,
			)
		}
	case <-time.After(CANCEL_BUDGET):
		t.Fatal("the waiting run did not stop")
	}
}

// TestRun_FirstRunsAtOnceShareOnePureModule checks that runs which start
// together, before any has built a pure module, get one value between them.
// Run it with -race.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func TestRun_FirstRunsAtOnceShareOnePureModule(t *testing.T) {
	var (
		built = _Linked(
			t,
			_Sources{
				MAIN:     KEEPS,
				LOADABLE: PURE,
			},
		)
		start  = make(chan struct{})
		values = make(
			[]starlark.Value,
			RUNS_AT_ONCE,
		)
		errs = make(
			[]error,
			RUNS_AT_ONCE,
		)
		group sync.WaitGroup
	)

	for idx := range RUNS_AT_ONCE {
		group.Go(func() {
			<-start

			values[idx], errs[idx] = artifact.Run(
				t.Context(),
				built,
			)
		})
	}

	close(start)
	group.Wait()

	for idx := range RUNS_AT_ONCE {
		if errs[idx] != nil {
			t.Fatalf(
				"run %d: %v",
				idx,
				errs[idx],
			)
		}

		if values[idx] != values[0] {
			t.Fatalf(
				"run %d got its own value, want the one every run shares",
				idx,
			)
		}
	}
}
