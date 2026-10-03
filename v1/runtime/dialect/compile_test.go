package dialect_test

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	SCRIPT = "taken.star"
	ENTRY  = "main"
	KEEP   = "keep"
	BEFORE = "referenced before assignment"
)

// _Held is a spawn that starts nothing. It keeps each function it is handed,
// so a test calls them after the loop that made them has moved on, which is
// the moment a variable read late and a variable taken early disagree. keep
// is the same builtin under a name the dialect does not rewrite.
type _Held struct {
	fns []starlark.Callable
}

// _Keep holds the one function it is handed.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func (h *_Held) _Keep(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var target starlark.Callable

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &target)
	if err != nil {
		return nil, err
	}

	h.fns = append(h.fns, target)

	return starlark.None, nil
}

// _Ran compiles src, runs its main, then calls every function main handed to
// spawn or keep, and returns what each gave back.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
//   - 2026-10-02 01:18: predeclares CALL, which the dialect now compiles a
//     statement call of a script function through
func _Ran(t *testing.T, src string) ([]string, error) {
	t.Helper()

	held := &_Held{}

	env := starlark.StringDict{
		dialect.SPAWN: starlark.NewBuiltin(dialect.SPAWN, held._Keep),
		KEEP:          starlark.NewBuiltin(KEEP, held._Keep),
		spelling.CALL: scheduler.CALL,
	}

	_, program, err := dialect.Compile(SCRIPT, []byte(src), env.Has)
	if err != nil {
		return nil, err
	}

	thread := &starlark.Thread{Name: SCRIPT}

	globals, err := program.Init(thread, env)
	if err != nil {
		return nil, err
	}

	_, err = starlark.Call(thread, globals[ENTRY], nil, nil)
	if err != nil {
		return nil, err
	}

	got := make([]string, 0, len(held.fns))

	for _, fn := range held.fns {
		var value starlark.Value

		value, err = starlark.Call(thread, fn, nil, nil)
		if err != nil {
			return nil, err
		}

		got = append(got, value.String())
	}

	return got, nil
}

// _Same fails unless a run gave back exactly want.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func _Same(t *testing.T, got []string, err error, want ...string) {
	t.Helper()

	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestCompile_TakesALoopVariableAtTheSpawn is the bug this rule exists for: a
// thread per item, each reading the one variable the loop goes on writing.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_TakesALoopVariableAtTheSpawn(t *testing.T) {
	got, err := _Ran(t, `
def main():
    for x in range(3):
        spawn(lambda: x * 10)
`)

	_Same(t, got, err, "0", "10", "20")
}

// TestCompile_LeavesALambdaElsewhereAlone proves the rule is spawn's and not
// every lambda's: the same loop handing its lambdas to anything else reads x
// late, as Starlark always has.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_LeavesALambdaElsewhereAlone(t *testing.T) {
	got, err := _Ran(t, `
def main():
    for x in range(3):
        keep(lambda: x * 10)
`)

	_Same(t, got, err, "20", "20", "20")
}

// TestCompile_LeavesAScriptsOwnSpawnAlone proves the rule follows the builtin
// and not the word: a script that binds spawn itself means its own function.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_LeavesAScriptsOwnSpawnAlone(t *testing.T) {
	got, err := _Ran(t, `
def spawn(fn):
    keep(fn)

def main():
    for x in range(3):
        spawn(lambda: x * 10)
`)

	_Same(t, got, err, "20", "20", "20")
}

// TestCompile_TakesEveryVariableTheLambdaReads proves a lambda reading two
// variables takes both, each at the spawn.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_TakesEveryVariableTheLambdaReads(t *testing.T) {
	got, err := _Ran(t, `
def main():
    for x in range(2):
        for y in range(2):
            spawn(lambda: (x, y))
`)

	_Same(t, got, err, "(0, 0)", "(0, 1)", "(1, 0)", "(1, 1)")
}

// TestCompile_TakesAVariableOfAnOuterFunction proves a variable reached
// through a function between the lambda and its owner is taken too.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_TakesAVariableOfAnOuterFunction(t *testing.T) {
	got, err := _Ran(t, `
def main():
    for x in range(3):
        def inner():
            spawn(lambda: x * 10)

        inner()
`)

	_Same(t, got, err, "0", "10", "20")
}

// TestCompile_LeavesALambdaTakingStarArgsAlone proves the one lambda the rule
// cannot rewrite is left as written: nothing may follow *args, and spawn
// refuses a function taking it anyway.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_LeavesALambdaTakingStarArgsAlone(t *testing.T) {
	got, err := _Ran(t, `
def main():
    for x in range(3):
        spawn(lambda *rest: x * 10)
`)

	_Same(t, got, err, "20", "20", "20")
}

// TestCompile_FailsAtTheSpawnForAVariableNotYetAssigned proves a variable read
// before it is assigned fails where the lambda is written, every time, rather
// than on whichever side of the assignment a thread happened to run.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_FailsAtTheSpawnForAVariableNotYetAssigned(t *testing.T) {
	_, err := _Ran(t, `
def main():
    spawn(lambda: y)
    y = 1
`)
	if err == nil || !strings.Contains(err.Error(), BEFORE) {
		t.Fatalf("got %v, want a failure saying %q", err, BEFORE)
	}
}

// TestCompile_ReturnsTheTreeAsWritten proves what a check reads is the source
// as written, not the rewritten lambda.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_ReturnsTheTreeAsWritten(t *testing.T) {
	src := `
def main():
    x = 1
    spawn(lambda: x)
`

	env := starlark.StringDict{dialect.SPAWN: starlark.None}

	tree, _, err := dialect.Compile(SCRIPT, []byte(src), env.Has)
	if err != nil {
		t.Fatal(err)
	}

	syntax.Walk(tree, func(node syntax.Node) bool {
		lambda, ok := node.(*syntax.LambdaExpr)
		if ok && len(lambda.Params) > 0 {
			t.Fatalf("the returned lambda has %d parameters, want none",
				len(lambda.Params))
		}

		return true
	})
}

// TestCompile_KeepsTheInterpretersPosition proves a resolve error arrives as
// the interpreter wrote it, opening with the file, line and column.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCompile_KeepsTheInterpretersPosition(t *testing.T) {
	_, err := _Ran(t, `
def main():
    return missing
`)
	if err == nil || !strings.HasPrefix(err.Error(), SCRIPT+":3:") {
		t.Fatalf("got %v, want an error opening with %s:3:", err, SCRIPT)
	}
}
