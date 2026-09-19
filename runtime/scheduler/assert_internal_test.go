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
// Revisions:
//   - 2026-09-19 22:21: initial creation
func TestAssert_UsesStarlarkTruthiness(t *testing.T) {
	falsey := []starlark.Value{
		starlark.MakeInt(0),
		starlark.String(""),
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

// TestAssert_EndsOnlyTheThreadItRanOn proves the rule that makes an isolated
// failure possible: a Starlark error unwinds the thread it was raised on and no
// other, so a failed assertion in a spawned function leaves its siblings alone
// and surfaces where somebody joins it.
//
// This is the first test that puts assert, spawn and join together, and it is
// the reason assert is a builtin at all rather than a Go-side check.
//
// Revisions:
//   - 2026-09-19 22:22: initial creation
func TestAssert_EndsOnlyTheThreadItRanOn(t *testing.T) {
	const source = `
def isolated():
    assert(False, "two and two")
    return 1

def survivor():
    return 7
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

	failing, err := _Spawned(t, run, globals[ISOLATED])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	fine, err := _Spawned(t, run, globals[SURVIVOR])
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	<-fine.done

	if fine.err != nil {
		t.Fatalf("a sibling thread was ended by another's assertion: %v", fine.err)
	}

	_, err = _Joined(run, failing)
	if !errors.Is(err, ErrAssert) {
		t.Fatalf("joining the failed thread gave %v, want ErrAssert", err)
	}

	t.Logf("one thread failed, the other did not: %v", err)
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
