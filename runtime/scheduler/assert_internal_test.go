// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: _Assert is an unexported builtin, and the tests that matter here call it
// through a script that also spawns and joins, which needs a run on the thread
// that only a later phase creates.
package scheduler

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
)

const (
	ASSERT_SCRIPT = "assert_test.star"
	WHY           = "two and two"
	ISOLATED      = "isolated"
	SURVIVOR      = "survivor"
	EXPECTED_NAME = "scheduler"
	EXPECTED_SIZE = 4
)

// _Call invokes _Assert with the arguments given.
//
// Revisions:
//   - 2026-09-19 22:19: initial creation
func _Call(args ...starlark.Value) (starlark.Value, error) {
	return _Assert(&starlark.Thread{Name: ASSERT_SCRIPT}, nil, starlark.Tuple(args), nil)
}

// TestAssert_DoesNothingWhenTheConditionHolds proves a true assertion is not an
// event: it returns None and the thread carries on.
//
// Revisions:
//   - 2026-09-19 22:20: initial creation
func TestAssert_DoesNothingWhenTheConditionHolds(t *testing.T) {
	value, err := _Call(starlark.True, starlark.String(WHY))
	if err != nil {
		t.Fatalf("a true assertion failed: %v", err)
	}

	if value != starlark.None {
		t.Fatalf("returned %v, want None", value)
	}
}

// TestAssert_FailsWhenItDoesNot proves a false condition raises, carrying the
// message, reachable through errors.Is.
//
// Revisions:
//   - 2026-09-19 22:20: initial creation
func TestAssert_FailsWhenItDoesNot(t *testing.T) {
	_, err := _Call(starlark.False, starlark.String(WHY))
	if !errors.Is(err, ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}

	if !strings.Contains(err.Error(), WHY) {
		t.Fatalf("the failure does not carry the message: %v", err)
	}

	t.Logf("refused: %v", err)
}

// TestAssert_UsesStarlarkTruthiness proves the condition is judged the way the
// language's own `if` judges it, rather than by a rule of this package's.
//
// A lone string is not in this table: it is refused outright, because it reads
// like a message and would otherwise be judged as a condition. That is its own
// test below.
//
// Revisions:
//   - 2026-09-19 22:21: initial creation
//   - 2026-09-20 00:01: a lone string is no longer a condition
func TestAssert_UsesStarlarkTruthiness(t *testing.T) {
	falsey := []starlark.Value{
		starlark.MakeInt(0),
		starlark.NewList(nil),
		starlark.None,
	}

	for _, value := range falsey {
		_, err := _Call(value)
		if !errors.Is(err, ErrAssert) {
			t.Fatalf("assert(%v) gave %v, want ErrAssert", value, err)
		}
	}

	_, err := _Call(starlark.MakeInt(1))
	if err != nil {
		t.Fatalf("assert(1) failed: %v", err)
	}
}

// TestAssert_RefusesALoneString proves the trap is closed: assert("text") reads
// like an unconditional failure and used to behave like a passing test, because
// a non-empty string is true.
//
// Revisions:
//   - 2026-09-20 00:02: initial creation
func TestAssert_RefusesALoneString(t *testing.T) {
	for _, text := range []string{"this should never happen", ""} {
		_, err := _Call(starlark.String(text))
		if !errors.Is(err, ErrNotACondition) {
			t.Fatalf("assert(%q) gave %v, want ErrNotACondition", text, err)
		}
	}

	// Two arguments is the ordinary shape, so the refusal does not apply: the
	// first is a condition, and a non-empty string is a true one.
	_, err := _Call(starlark.String("a truthy condition"), starlark.String(WHY))
	if err != nil {
		t.Fatalf("a string condition with a message gave %v, want it to pass", err)
	}

	_, err = _Call(starlark.String(""), starlark.String(WHY))
	if !errors.Is(err, ErrAssert) {
		t.Fatalf("an empty string condition gave %v, want ErrAssert", err)
	}
}

// TestAssert_KeywordMessageAlwaysFails proves the shape plan.md asks for:
// assert(msg = "...") with no condition is an unconditional failure, and a
// keyword cannot be mistaken for something to test.
//
// Revisions:
//   - 2026-09-20 00:03: initial creation
func TestAssert_KeywordMessageAlwaysFails(t *testing.T) {
	kwargs := []starlark.Tuple{{starlark.String("msg"), starlark.String(WHY)}}

	_, err := _Assert(&starlark.Thread{Name: ASSERT_SCRIPT}, nil, starlark.Tuple{}, kwargs)
	if !errors.Is(err, ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}

	if !strings.Contains(err.Error(), WHY) {
		t.Fatalf("the failure does not carry the message: %v", err)
	}

	t.Logf("unconditional: %v", err)
}

// TestAssert_MessageIsOptional proves a bare assertion is legal and reaches the
// sentinel without one.
//
// Revisions:
//   - 2026-09-19 22:21: initial creation
func TestAssert_MessageIsOptional(t *testing.T) {
	_, err := _Call(starlark.False)
	if !errors.Is(err, ErrAssert) {
		t.Fatalf("got %v, want ErrAssert", err)
	}
}

// TestAssert_StopsTheWholeRun proves a failed assertion is not local to the
// thread that made it: the spine and every spawned thread stop, whether or not
// anyone joins the failed handle.
//
// This reverses what this package did until 2026-09-20 00:09, when a failed
// assertion ended one thread and its siblings ran on. The earlier behaviour is
// what a library wants; this is what a test runner wants, and this runtime
// runs tests.
//
// The sibling here counts to a hundred million. If the assertion did not reach
// it, this test would take seconds rather than milliseconds.
//
// Revisions:
//   - 2026-09-20 00:14: initial creation, replacing TestAssert_EndsOnlyTheThreadItRanOn
func TestAssert_StopsTheWholeRun(t *testing.T) {
	const source = `
def isolated():
    assert(False, "two and two")
    return 1

def survivor():
    total = 0
    for i in range(100000000):
        total += i
    return total
`

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: ASSERT_SCRIPT},
		ASSERT_SCRIPT,
		[]byte(source),
		(&Builtins{}).Values(),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	globals.Freeze()

	run := _Started(t.Context())

	spinner, err := _Spawned(t, run, globals[SURVIVOR])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	failing, err := _Spawned(t, run, globals[ISOLATED])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	<-failing.done

	// The run's outcome, not the handle's error. The asserting thread is
	// cancelled by its own assertion, so its handle reports cancellation like
	// any other - which is why the outcome is kept somewhere a shutdown cannot
	// overwrite.
	if run._Outcome() == nil {
		t.Fatal("the run does not know why it ended")
	}

	if !errors.Is(run._Outcome(), ErrAssert) {
		t.Fatalf("the run ended with %v, want ErrAssert", run._Outcome())
	}

	// The sibling counts to a hundred million. Waiting for it is the whole
	// check: if the assertion did not reach it, this blocks for seconds and
	// the budget below fails the test.
	select {
	case <-spinner.done:
	case <-time.After(JOIN_LIMIT):
		t.Fatal("a sibling thread ran on after the assertion")
	}

	if !errors.Is(spinner.err, ErrCancelled) {
		t.Fatalf("the sibling ended with %v, want ErrCancelled", spinner.err)
	}

	t.Logf("outcome: %v / sibling: %v", run._Outcome(), spinner.err)
}

// TestBuiltins_SuppliesTheFourNames proves what a host gets, and that each call
// builds a fresh map rather than handing out one shared with every other host.
//
// Revisions:
//   - 2026-09-19 22:23: initial creation
func TestBuiltins_SuppliesTheFourNames(t *testing.T) {
	plugin := &Builtins{}

	if plugin.Name() != EXPECTED_NAME {
		t.Fatalf("plugin is called %q, want %q", plugin.Name(), EXPECTED_NAME)
	}

	values := plugin.Values()

	if len(values) != EXPECTED_SIZE {
		t.Fatalf("supplies %v, want %d names", values.Keys(), EXPECTED_SIZE)
	}

	for _, name := range []string{SPAWN, JOIN, CANCEL, ASSERT} {
		builtin, ok := values[name].(*starlark.Builtin)
		if !ok {
			t.Fatalf("%s is %T, want *starlark.Builtin", name, values[name])
		}

		if builtin.Name() != name {
			t.Fatalf("%s is named %q", name, builtin.Name())
		}
	}

	values[SPAWN] = starlark.None

	if plugin.Values()[SPAWN] == starlark.None {
		t.Fatal("changing one host's environment changed everybody's")
	}
}
