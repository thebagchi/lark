// Probes for change C28: outcomes(h, ...) waits for every handle and returns
// one dict per handle, in argument order, without raising and without
// cancelling the handles it has not reached.
//
// The decision (plan.md section 6, Q2, "Continuing after an ignored error"):
// join is unchanged; outcomes does not cancel siblings; a record is
// {"ok", "value", "error", "kind"} with kind one of "fail", "cancelled",
// "cycle" and "nested"; the error is the whole text; a cancelled handle is a
// record; an assert in a handle still ends the run.
//
// Revisions:
//   - 2026-11-02 17:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// OUTCOMES_HEAD defines the threads the probes spawn.
	//
	// Revisions:
	//   - 2026-11-02 17:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	OUTCOMES_HEAD = `
def good():
    return 7

def bad():
    fail("boom")

def slow():
    sleep(100)
    return "late"

def spin():
    for i in range(100000000):
        pass

def asserts():
    assert(False, "run ends")

def searcher():
    x = []
    x.append(x)
    return find_key(x, "k")

def shape(rs):
    return [(r["ok"], r["kind"], r["value"]) for r in rs]
`

	// LONG_ERROR is how many bytes the long-text probe fails with.
	//
	// Revisions:
	//   - 2026-11-02 17:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LONG_ERROR = 5000
)

// TestOutcomes_ReturnsOneRecordPerHandleInArgumentOrder proves a mix of ok,
// fail and cancelled handles comes back as dicts, in the order given.
//
// Starting condition: no plugins beyond the defaults.
// Success: the three records read (ok, kind, value) as listed.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-04 06:39: an error text begins with its position (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_ReturnsOneRecordPerHandleInArgumentOrder(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    a = spawn(bad)
    b = spawn(good)
    c = spawn(spin)
    cancel(c)
    rs = outcomes(a, b, c)
    return (type(rs), type(rs[0]), shape(rs), rs[0]["error"], rs[1]["error"])
`,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want no error",
			err,
		)
	}

	want := `("list", "dict", [(False, "fail", None), (True, None, 7), ` +
		`(False, "cancelled", None)], "probe.star:6: fail: boom", None)`
	if got.String() != want {
		t.Fatalf(
			"got %s, want %s",
			got,
			want,
		)
	}
}

// TestOutcomes_DoesNotCancelSiblings proves a slow handle listed after a
// failure finishes ok, and that join in the same script does not.
//
// Success: outcomes gives ok for the slow handle; join fails the run.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_DoesNotCancelSiblings(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    rs = outcomes(spawn(bad), spawn(slow))
    return shape(rs)
`,
	)
	if err != nil {
		t.Fatalf(
			"outcomes: got %v, want no error",
			err,
		)
	}

	want := `[(False, "fail", None), (True, None, "late")]`
	if got.String() != want {
		t.Fatalf(
			"got %s, want %s",
			got,
			want,
		)
	}

	_, err = _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    return join(spawn(bad), spawn(slow))
`,
	)
	if err == nil {
		t.Fatal("join: got no error, want the failure")
	}
}

// TestOutcomes_AnAssertInAHandleEndsTheRun proves an assert-level error is
// not turned into a record.
//
// Success: the run ends with runtime.ErrAssert.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_AnAssertInAHandleEndsTheRun(t *testing.T) {
	t.Parallel()

	_, err := _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    return outcomes(spawn(asserts), spawn(slow))
`,
	)
	if !errors.Is(
		err,
		runtime.ErrAssert,
	) {
		t.Fatalf(
			"got %v, want ErrAssert",
			err,
		)
	}
}

// TestOutcomes_ACycleIsKindCycle proves a self-containing value in a handle
// gives kind "cycle", and the text of the cycle.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-04 06:39: an error text begins with its position (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func TestOutcomes_ACycleIsKindCycle(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    r = outcomes(spawn(searcher))[0]
    return (r["ok"], r["kind"], r["error"])
`,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want no error",
			err,
		)
	}

	want := `(False, "cycle", "probe.star:22: find_key: ` +
		trail.ErrCyclic.Error() +
		`")`
	if got.String() != want {
		t.Fatalf(
			"got %s, want %s",
			got,
			want,
		)
	}
}

// TestOutcomes_ANestedUpdateIsKindNested proves a state operation that an
// update's child makes after the update gives kind "nested".
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_ANestedUpdateIsKindNested(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def child():
    sleep(50)
    state.set("b", 1)

def main():
    found = []
    def change(v):
        found.append(spawn(child))
        return "done"
    state.update("a", change)
    r = outcomes(found[0])[0]
    return (r["ok"], r["kind"])
`,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want no error",
			err,
		)
	}

	if got.String() != `(False, "nested")` {
		t.Fatalf(
			"got %s, want (False, \"nested\")",
			got,
		)
	}
}

// TestOutcomes_KeepsTheWholeErrorText proves the error is not cut.
//
// Success: the record's error is the position, "fail: " and all LONG_ERROR bytes.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-04 06:39: an error text begins with its position (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_KeepsTheWholeErrorText(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def long():
    fail("x" * 5000)

def main():
    return outcomes(spawn(long))[0]["error"]
`,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want no error",
			err,
		)
	}

	text, ok := starlark.AsString(got)
	if !ok {
		t.Fatalf(
			"got %s, want a string",
			got,
		)
	}

	if text != "probe.star:3: fail: "+strings.Repeat(
		"x",
		LONG_ERROR,
	) {
		t.Fatalf(
			"error is %d bytes, want the whole text",
			len(text),
		)
	}
}

// TestOutcomes_Refusals proves the argument rules are join's: no handles is an
// empty list, a keyword or a non-handle is runtime.ErrHandle, and the builtin's
// name is said once.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrHandle, formerly ErrNotAHandle (C121, Q348)
func TestOutcomes_Refusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call string
		want string
	}{
		{
			name: "no handles",
			call: "outcomes()",
			want: "[]",
		},
		{
			name: "no handles, as join",
			call: "join()",
			want: "[]",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				got, err := _Value(
					t,
					"def main():\n    return "+test.call+"\n",
				)
				if err != nil {
					t.Fatalf(
						"got %v, want no error",
						err,
					)
				}

				if got.String() != test.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						test.want,
					)
				}
			},
		)
	}

	refused := []struct {
		name string
		call string
		want string
	}{
		{
			name: "a number",
			call: "outcomes(1)",
			want: "outcomes got int",
		},
		{
			name: "a number after a handle",
			call: "outcomes(spawn(good), 'x')",
			want: "outcomes got string",
		},
		{
			name: "a keyword",
			call: "outcomes(spawn(good), a = 1)",
			want: "outcomes takes no keyword arguments",
		},
	}

	for _, test := range refused {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _Value(
					t,
					OUTCOMES_HEAD+"def main():\n    return "+test.call+"\n",
				)
				if !errors.Is(
					err,
					core.ErrHandle,
				) {
					t.Fatalf(
						"got %v, want ErrHandle",
						err,
					)
				}

				if strings.Count(
					err.Error(),
					"outcomes",
				) != 1 || !strings.Contains(
					err.Error(),
					test.want,
				) {
					t.Fatalf(
						"text %q, want %q with the name said once",
						err,
						test.want,
					)
				}
			},
		)
	}
}

// TestOutcomes_UnderTheLockIsRefused proves a wait inside an update is refused
// as join's is, with the builtin's name said once.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_UnderTheLockIsRefused(t *testing.T) {
	t.Parallel()

	_, err := _Value(
		t,
		OUTCOMES_HEAD+`
def main():
    h = spawn(good)
    return state.update("k", lambda v: outcomes(h))
`,
	)
	if !errors.Is(
		err,
		runtime.ErrNested,
	) {
		t.Fatalf(
			"got %v, want ErrNested",
			err,
		)
	}

	if strings.Count(
		err.Error(),
		"outcomes",
	) != 1 {
		t.Fatalf(
			"text %q, want outcomes said once",
			err,
		)
	}
}

// TestOutcomes_AScriptMayDefineTheName proves a global, a parameter and a local
// named outcomes still work: a script global shadows a predeclared name.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C28)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_AScriptMayDefineTheName(t *testing.T) {
	t.Parallel()

	got, err := _Value(
		t,
		`
def outcomes(x):
    return ("mine", x)

def local(y):
    outcomes = y + 1
    return outcomes

def main():
    return (outcomes(1), local(1))
`,
	)
	if err != nil {
		t.Fatalf(
			"got %v, want no error",
			err,
		)
	}

	if got.String() != `(("mine", 1), 2)` {
		t.Fatalf(
			"got %s",
			got,
		)
	}
}
