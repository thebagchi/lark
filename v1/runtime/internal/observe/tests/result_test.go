package tests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// ORDERED prints from the spine, from a thread the spine joins, and from
	// the spine again. HALFWAY prints one line and then raises. EXPLODES calls
	// a builtin that panics, and WHERE_FIXTURE returns the stack its main runs
	// on.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: hold the script text, inlined from testdata (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ORDERED = `
# Prints from the spine, then from a thread the spine joins, then from the
# spine again, so the order of the three lines is fixed.

def alpha():
    print("two")

def main():
    print("one")
    join(spawn(alpha))
    print("three")
`
	HALFWAY = `
# Prints one line and then raises, so a failed run still has a line to show.

def main():
    print("before")
    fail("after the print")
`
	EXPLODES = `
# Calls a builtin that panics, on a thread the spine joins.

def boom():
    explode()
    return 1

def main():
    return join(spawn(boom))
`
	WHERE_FIXTURE = `
# Returns the stack of the goroutine its main runs on.

def main():
    return where()
`

	// ORDERED_PRINTS is how many lines ORDERED prints.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ORDERED_PRINTS = 3

	// FIRST_THREAD is the id of the first thread the spine spawns.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
	//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FIRST_THREAD = common.THREAD + "1"

	// LEVEL is where a line WithLog writes stops saying when it was written,
	// so two runs' lines compare equal.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LEVEL = "level="

	// SETTLE_TRIES and SETTLE_WAIT bound the wait for the goroutines a run
	// started to exit after it returned.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SETTLE_TRIES = 200
	SETTLE_WAIT  = 5 * time.Millisecond
)

// TestEval_EndsAsStartAndWaitEnd checks that a run evaluated on the
// caller's goroutine ends as a started run waited for does: the same value,
// the same graph, and the same error, matched by errors.Is and by its text.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_EndsAsStartAndWaitEnd; uses Eval, formerly
//     Evaluate (C121, Q350)
func TestEval_EndsAsStartAndWaitEnd(t *testing.T) {
	broken := func(change *workflowpb.Change) {
		panic("watcher broke")
	}

	var (
		probe = artifact.WithPlugins(new(_Probe))
		tests = []struct {
			name   string
			art    *artifact.Artifact
			opts   []common.RunOption
			target error
		}{
			{
				name: "a value",
				art: _Compile(
					t,
					QUICK,
				),
			},
			{
				name: "a spawn and a join",
				art: _Compile(
					t,
					PRINTS,
				),
			},
			{
				name: "an assertion",
				art: _Compile(
					t,
					FAILING,
				),
				target: core.ErrAssert,
			},
			{
				name: "a fail",
				art: _Compile(
					t,
					RAISING,
				),
			},
			{
				name: "a builtin that panics",
				art: _Compile(
					t,
					EXPLODES,
					probe,
				),
			},
			{
				name: "a watcher that panics",
				art: _Compile(
					t,
					QUICK,
				),
				opts: []common.RunOption{
					artifact.WithChanges(broken),
				},
			},
			{
				name: "no artifact",
			},
		}
	)

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				run, err := observe.Start(
					t.Context(),
					tt.art,
					tt.opts...,
				)
				if err != nil {
					t.Fatal(err)
				}

				var (
					value, waited = run.Wait()
				)

				result, err := observe.Eval(
					t.Context(),
					tt.art,
					tt.opts...,
				)

				_Same(
					t,
					waited,
					err,
				)
				_Matched(
					t,
					tt.target,
					err,
				)

				if result == nil {
					t.Fatal("want a result, also for a run that failed")
				}

				_Valued(
					t,
					value,
					result.Value,
					err,
				)

				if !proto.Equal(
					result.Graph,
					run.Status(),
				) {
					t.Fatalf(
						"graph\n%v\nwant\n%v",
						result.Graph,
						run.Status(),
					)
				}
			},
		)
	}
}

// TestEval_KeepsEachPrintInOrder checks that a run's result holds each line
// its script printed, with the thread and the function that printed it, in the
// order printed, and that WithLog still writes the lines a started run writes.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_KeepsEachPrintInOrder; uses Eval, formerly
//     Evaluate (C121, Q350)
func TestEval_KeepsEachPrintInOrder(t *testing.T) {
	var (
		started   bytes.Buffer
		evaluated bytes.Buffer
		built     = _Compile(
			t,
			ORDERED,
		)
		want = []*common.Print{
			{
				Thread:   common.SPINE,
				Function: ENTRY,
				Text:     "one",
			},
			{
				Thread:   FIRST_THREAD,
				Function: "alpha",
				Text:     "two",
			},
			{
				Thread:   common.SPINE,
				Function: ENTRY,
				Text:     "three",
			},
		}
	)

	run, err := observe.Start(
		t.Context(),
		built,
		artifact.WithLog(&started),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	result, err := observe.Eval(
		t.Context(),
		built,
		artifact.WithLog(&evaluated),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(
		result.Prints,
		want,
	) {
		t.Fatalf(
			"prints %s, want %s",
			_Shown(result.Prints),
			_Shown(want),
		)
	}

	var (
		logged = _Untimed(evaluated.String())
		wanted = _Untimed(started.String())
	)

	if !slices.Equal(
		logged,
		wanted,
	) {
		t.Fatalf(
			"WithLog wrote %q, a started run wrote %q",
			logged,
			wanted,
		)
	}
}

// TestEval_KeepsAPrintTheLoggerDrops checks that a print reaches the
// result whatever level the logger writes at, and that the logger is still
// told only what it asked for. WithLog has no level, so the test gives the run
// an option of its own that sets a logger at warn. It touches no global state,
// so it may run in parallel.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: gives the run a logger at warn through an option of its own,
//     since WithLogger is dropped and WithLog has no level; renamed from
//     TestEvaluate_KeepsAPrintTheLoggerDrops; uses Eval, formerly Evaluate (C121, Q340, Q350)
func TestEval_KeepsAPrintTheLoggerDrops(t *testing.T) {
	var (
		out    bytes.Buffer
		logger = slog.New(slog.NewTextHandler(
			&out,
			&slog.HandlerOptions{
				Level: slog.LevelWarn,
			},
		))
	)

	result, err := observe.Eval(
		t.Context(),
		_Compile(
			t,
			ORDERED,
		),
		func(given *common.Given) {
			given.Logger = logger
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if out.Len() != 0 {
		t.Fatalf(
			"a logger at warn wrote %q",
			out.String(),
		)
	}

	if len(result.Prints) != ORDERED_PRINTS {
		t.Fatalf(
			"want %d prints, got %s",
			ORDERED_PRINTS,
			_Shown(result.Prints),
		)
	}
}

// TestEval_AFailedRunKeepsWhatItDid checks that a run that fails still
// returns its result: no value, the graph as the run ended, and each line it
// printed before it failed.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_AFailedRunKeepsWhatItDid; uses Eval,
//     formerly Evaluate (C121, Q350)
func TestEval_AFailedRunKeepsWhatItDid(t *testing.T) {
	result, err := observe.Eval(
		t.Context(),
		_Compile(
			t,
			HALFWAY,
		),
	)
	if err == nil {
		t.Fatal("want the run to fail")
	}

	if result == nil {
		t.Fatal("want a result for a failed run")
	}

	if result.Value != nil {
		t.Fatalf(
			"want no value, got %v",
			result.Value,
		)
	}

	if result.Graph.GetStatus() != workflowpb.Status_STATUS_FAILED {
		t.Fatalf(
			"want a failed graph, got %v",
			result.Graph.GetStatus(),
		)
	}

	want := []*common.Print{
		{
			Thread:   common.SPINE,
			Function: ENTRY,
			Text:     "before",
		},
	}
	if !reflect.DeepEqual(
		result.Prints,
		want,
	) {
		t.Fatalf(
			"prints %s, want %s",
			_Shown(result.Prints),
			_Shown(want),
		)
	}
}

// TestEval_RunsOnTheCallersGoroutine checks that the script's main runs on
// the goroutine that called Eval, where a started run's main runs on one
// of its own.
//
// The stack a builtin reads holds this test's frame only when nothing started
// a goroutine between the test and the script.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/, where the frame is named tests (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_RunsOnTheCallersGoroutine; uses Eval,
//     formerly Evaluate (C121, Q350)
func TestEval_RunsOnTheCallersGoroutine(t *testing.T) {
	var (
		frame = "tests.TestEval_RunsOnTheCallersGoroutine("
		built = _Compile(
			t,
			WHERE_FIXTURE,
			artifact.WithPlugins(new(_Probe)),
		)
	)

	result, err := observe.Eval(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		_Text(result.Value),
		frame,
	) {
		t.Fatalf(
			"main ran on a goroutine of its own:\n%s",
			_Text(result.Value),
		)
	}

	run, err := observe.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(
		_Text(value),
		frame,
	) {
		t.Fatal("a started run's main ran on the caller's goroutine")
	}
}

// TestEval_StopsWhenCancelled checks that cancelling the caller's context
// stops a run on the caller's goroutine, as it stops a started one.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_StopsWhenCancelled; uses Eval, formerly
//     Evaluate (C121, Q350)
func TestEval_StopsWhenCancelled(t *testing.T) {
	ctx, stop := context.WithTimeout(
		t.Context(),
		SETTLE,
	)
	defer stop()

	var (
		begun       = time.Now()
		result, err = observe.Eval(
			ctx,
			_Compile(
				t,
				SLOW,
			),
		)
		taken = time.Since(begun)
	)

	if !errors.Is(
		err,
		scheduler.ErrCancelled,
	) {
		t.Fatalf(
			"want ErrCancelled, got %v",
			err,
		)
	}

	if taken > PATIENCE {
		t.Fatalf(
			"a cancelled run took %s",
			taken,
		)
	}

	if result.Graph.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf(
			"want a cancelled graph, got %v",
			result.Graph.GetStatus(),
		)
	}
}

// TestEval_LeavesNoGoroutineBehind checks that once Eval returns, every
// goroutine the run started has exited.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_LeavesNoGoroutineBehind; uses Eval, formerly
//     Evaluate (C121, Q350)
func TestEval_LeavesNoGoroutineBehind(t *testing.T) {
	var (
		built = _Compile(
			t,
			PRINTS,
		)
		before = runtime.NumGoroutine()
	)

	_, err := observe.Eval(
		t.Context(),
		built,
		artifact.WithLog(new(bytes.Buffer)),
	)
	if err != nil {
		t.Fatal(err)
	}

	for range SETTLE_TRIES {
		if runtime.NumGoroutine() <= before {
			return
		}

		time.Sleep(SETTLE_WAIT)
	}

	t.Fatalf(
		"goroutines did not settle: %d before, %d after",
		before,
		runtime.NumGoroutine(),
	)
}

// TestEval_SendsItsChanges checks that WithChanges still works for a run
// on the caller's goroutine: the changes, patched into {}, are the graph the
// result holds.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestEvaluate_SendsItsChanges; uses Eval, formerly
//     Evaluate (C121, Q350)
func TestEval_SendsItsChanges(t *testing.T) {
	var (
		guard   sync.Mutex
		changes []*workflowpb.Change
		watch   = func(change *workflowpb.Change) {
			guard.Lock()
			defer guard.Unlock()

			changes = append(
				changes,
				change,
			)
		}
		result, err = observe.Eval(
			t.Context(),
			_Compile(
				t,
				WATCHED,
			),
			artifact.WithChanges(watch),
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	guard.Lock()
	defer guard.Unlock()

	var held any = map[string]any{}

	for _, change := range changes {
		held = _Folded(
			t,
			held,
			change,
		)
	}

	want := _Decoded(
		t,
		result.Graph,
	)
	if !reflect.DeepEqual(
		held,
		want,
	) {
		t.Fatalf(
			"patched into\n%v\nwant\n%v",
			held,
			want,
		)
	}
}

// _Same fails t unless got is the error want is, by its text, and both are nil
// or neither is.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Same(
	t *testing.T,
	want error,
	got error,
) {
	t.Helper()

	if (want == nil) != (got == nil) {
		t.Fatalf(
			"got error %v, want %v",
			got,
			want,
		)
	}

	if want != nil && want.Error() != got.Error() {
		t.Fatalf(
			"got error %q, want %q",
			got,
			want,
		)
	}
}

// _Matched fails t unless err matches target with errors.Is, when there is a
// target.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Matched(
	t *testing.T,
	target error,
	err error,
) {
	t.Helper()

	if target == nil {
		return
	}

	if !errors.Is(
		err,
		target,
	) {
		t.Fatalf(
			"got %v, want it to match %v",
			err,
			target,
		)
	}
}

// _Valued fails t unless got is want for a run that succeeded, and nil for a
// run that failed with err.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Valued(
	t *testing.T,
	want starlark.Value,
	got starlark.Value,
	err error,
) {
	t.Helper()

	if err != nil {
		if got != nil {
			t.Fatalf(
				"want no value for a failed run, got %v",
				got,
			)
		}

		return
	}

	if _Text(got) != _Text(want) {
		t.Fatalf(
			"got %s, want %s",
			_Text(got),
			_Text(want),
		)
	}
}

// _Text is value as a script would print it, or empty for no value.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Text(value starlark.Value) string {
	if value == nil {
		return ""
	}

	text, ok := starlark.AsString(value)
	if ok {
		return text
	}

	return value.String()
}

// _Shown is prints as one readable line, for a failure message.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Shown(prints []*common.Print) string {
	var builder strings.Builder

	for _, line := range prints {
		builder.WriteString(line.Thread)
		builder.WriteString("/")
		builder.WriteString(line.Function)
		builder.WriteString(": ")
		builder.WriteString(line.Text)
		builder.WriteString("; ")
	}

	return builder.String()
}

// _Untimed is each line WithLog wrote to out, from its level on, since the
// time before it differs between two runs.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Untimed(out string) []string {
	var lines []string

	for line := range strings.Lines(out) {
		_, rest, found := strings.Cut(
			line,
			LEVEL,
		)
		if !found {
			rest = line
		}

		lines = append(
			lines,
			rest,
		)
	}

	return lines
}
