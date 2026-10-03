package dialect_test

import (
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// HIDDEN opens every name the dialect writes and no script can.
	HIDDEN = "%"

	// LIBRARY is the module the marked scripts load a function from.
	LIBRARY = "library.star"
)

// LOADED is what LIBRARY holds: one function a script may load and call.
const LOADED = `
def helper():
    pass
`

// _Record keeps what the calls the dialect changed did when they ran: each
// call that went through CALL, with the branch it was told, and each hidden
// keyword a builtin was passed.
type _Record struct {
	lines []string
}

// _Call is CALL as these tests see it: it keeps the function it is handed and
// the branch it is told, and calls the function without the hidden keyword.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func (r *_Record) _Call(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	builtin, rest := scheduler.Hidden(kwargs, spelling.BUILTIN)

	target, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%s handed a %s", fn.Name(), args[0].Type())
	}

	r.lines = append(r.lines, strings.TrimSpace("call "+target.Name()+" "+builtin))

	return starlark.Call(thread, target, args[1:], rest)
}

// _Hidden is a builtin that runs nothing and keeps the hidden keywords it was
// passed, after its own name.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func (r *_Record) _Hidden(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	line := fn.Name()

	for _, pair := range kwargs {
		key, named := starlark.AsString(pair[0])
		value, text := starlark.AsString(pair[1])

		hidden := named && text && strings.HasPrefix(key, HIDDEN)
		if hidden {
			line += " " + key + "=" + value
		}
	}

	r.lines = append(r.lines, line)

	return starlark.None, nil
}

// _Marked compiles src as the dialect does, runs its main, and returns what
// the calls the dialect changed did, in the order they ran.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func _Marked(t *testing.T, src string) []string {
	t.Helper()

	record := &_Record{}

	env := starlark.StringDict{
		spelling.CALL: starlark.NewBuiltin(spelling.CALL, record._Call),
	}

	for _, name := range []string{
		spelling.SPAWN,
		spelling.REPEAT,
		spelling.RETRY,
		spelling.TIMEOUT,
	} {
		env[name] = starlark.NewBuiltin(name, record._Hidden)
	}

	_, program, err := dialect.Compile(SCRIPT, []byte(src), env.Has)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	thread := &starlark.Thread{Name: SCRIPT, Load: _Library}

	globals, err := program.Init(thread, env)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	_, err = starlark.Call(thread, globals[ENTRY], nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	return record.lines
}

// _Library loads LIBRARY, the one module a marked script loads.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func _Library(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	return starlark.ExecFileOptions(dialect.OPTIONS, thread, module, LOADED, nil)
}

// TestMark_ReportsAStatementCallOfAScriptFunction checks which calls go
// through CALL: a call of a function the script defines or loads, alone or
// bound to one name, at the top level or in any function. A call inside an
// expression, of a builtin, of a nested def or of a parameter does not.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func TestMark_ReportsAStatementCallOfAScriptFunction(t *testing.T) {
	got := _Marked(t, `
load("library.star", "helper")

def once():
    return 1

def run(fn):
    fn()
    once()

def main():
    once()
    got = once()
    total = once() + got
    len(str(once()))
    helper()

    def inner():
        pass

    inner()
    run(once)

once()
`)

	want := []string{
		"call once",
		"call once",
		"call once",
		"call helper",
		"call run",
		"call once",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestMark_TellsABranchsCallItsBuiltin checks a call that is the whole of an
// if's branch passes if, and of a match's arm match, closer or not, while a
// branch of two statements, a loop in a branch and a match's subject do not.
// The last branch unpacks its arguments, and the keyword goes in before them.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func TestMark_TellsABranchsCallItsBuiltin(t *testing.T) {
	got := _Marked(t, `
def once():
    pass

def pick():
    return "b"

def main():
    if True:
        once()
    if False:
        pass
    elif True:
        once()
        pass
    if True:
        once()
        once()
    if True:
        for _ in range(1):
            once()
    _match = pick()
    if _match == "a":
        once()
    elif _match == "b":
        once()
    else:
        once()
    if True:
        once(*[])
`)

	want := []string{
		"call once if",
		"call once if",
		"call once",
		"call once",
		"call once",
		"call once match",
		"call once if",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestMark_PassesWhatABuiltinReports checks a spawn bound to a name passes the
// name, and a builtin that runs a function passes the function a lambda calls,
// wherever the call is written. A lambda calling no function of the script's
// names none.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func TestMark_PassesWhatABuiltinReports(t *testing.T) {
	got := _Marked(t, `
def work(n):
    pass

def main():
    h = spawn(work)
    spawn(lambda: work(1))
    held = spawn(lambda: work(2))
    spawn(lambda: len("x"))
    repeat(2, lambda: work(3))
    retry(2, lambda: work(4), delay = 5)
    timeout(5, lambda: work(5))
    handles = [spawn(lambda: work(i)) for i in range(1)]
    each = spawn(*[work])
`)

	want := []string{
		"spawn %binding=h",
		"spawn %callee=work",
		"spawn %binding=held %callee=work",
		"spawn",
		"repeat %callee=work",
		"retry %callee=work",
		"timeout %callee=work",
		"spawn %callee=work",
		"spawn %binding=each",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestMark_ReturnsTheTreeAsWritten checks the tree a check reads carries none
// of what the dialect compiles in: no CALL and no hidden keyword.
//
// Revisions:
//   - 2026-10-02 01:18: initial creation
func TestMark_ReturnsTheTreeAsWritten(t *testing.T) {
	src := `
def once():
    pass

def main():
    if True:
        once()
    h = spawn(lambda: once())
`

	env := starlark.StringDict{
		spelling.CALL:  starlark.None,
		spelling.SPAWN: starlark.None,
	}

	tree, _, err := dialect.Compile(SCRIPT, []byte(src), env.Has)
	if err != nil {
		t.Fatal(err)
	}

	syntax.Walk(tree, func(node syntax.Node) bool {
		name, ok := node.(*syntax.Ident)
		if ok && strings.HasPrefix(name.Name, HIDDEN) {
			t.Fatalf("the returned tree names %s", name.Name)
		}

		return true
	})
}
