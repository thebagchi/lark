package scheduler_test

import (
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// CALLED is a script the hidden call is tested against: one function taking a
// keyword, so a test can see that the hidden one did not reach it, and a
// lambda bound to a name, as a loaded module can hold one.
const CALLED = `
def greet(name = "world"):
    return "hello " + name

anonymous = lambda: 1
`

// TestCalling_ReportsAScriptFunction checks the hidden call reports a script
// function's call as a line of the thread that made it, with the branch it
// belongs to, and that the function never sees the hidden keyword.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
func TestCalling_ReportsAScriptFunction(t *testing.T) {
	into := new(_Lines)
	thread := &starlark.Thread{Name: "spine"}

	settings := &scheduler.Settings{Reporter: into}
	finish := scheduler.Begin(t.Context(), thread, spelling.ENTRY, settings)

	kwargs := []starlark.Tuple{
		{starlark.String(spelling.BUILTIN), starlark.String("if")},
		{starlark.String("name"), starlark.String("ada")},
	}

	value, err := starlark.Call(thread, scheduler.CALL, starlark.Tuple{_Greet(t)}, kwargs)

	err = finish(err)

	if err != nil {
		t.Fatalf("call: %v", err)
	}

	if value != starlark.String("hello ada") {
		t.Fatalf("got %v", value)
	}

	want := []string{
		"start thread_0 main ",
		"start thread_0 greet if",
		"end thread_0 greet",
		"end thread_0 main",
	}

	got := into._All()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestCalling_CallsABuiltinWithoutReporting checks a callee that is not a
// script function is called, and not reported.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
func TestCalling_CallsABuiltinWithoutReporting(t *testing.T) {
	into := new(_Lines)
	thread := &starlark.Thread{Name: "spine"}

	settings := &scheduler.Settings{Reporter: into}
	finish := scheduler.Begin(t.Context(), thread, spelling.ENTRY, settings)

	length := starlark.Universe["len"]
	args := starlark.Tuple{length, starlark.String("four")}

	value, err := starlark.Call(thread, scheduler.CALL, args, nil)

	err = finish(err)

	if err != nil || value != starlark.MakeInt(len("four")) {
		t.Fatalf("got %v, %v", value, err)
	}

	if got := into._All(); len(got) != 2 {
		t.Fatalf("want only main's own start and end, got %v", got)
	}
}

// TestCalling_CallsALambdaWithoutReporting checks a lambda is called and not
// reported: a loaded name can hold one, and no flow lists it.
//
// Revisions:
//   - 2026-10-02 01:20: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
func TestCalling_CallsALambdaWithoutReporting(t *testing.T) {
	into := new(_Lines)
	thread := &starlark.Thread{Name: "spine"}

	settings := &scheduler.Settings{Reporter: into}
	finish := scheduler.Begin(t.Context(), thread, spelling.ENTRY, settings)

	args := starlark.Tuple{_Named(t, "anonymous")}

	value, err := starlark.Call(thread, scheduler.CALL, args, nil)

	err = finish(err)

	if err != nil || value != starlark.MakeInt(1) {
		t.Fatalf("got %v, %v", value, err)
	}

	if got := into._All(); len(got) != 2 {
		t.Fatalf("want only main's own start and end, got %v", got)
	}
}

// TestHidden_TakesTheKeywordOut checks a hidden keyword is read and removed,
// and every other keyword kept in order.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
func TestHidden_TakesTheKeywordOut(t *testing.T) {
	kwargs := []starlark.Tuple{
		{starlark.String("a"), starlark.MakeInt(1)},
		{starlark.String(spelling.BINDING), starlark.String("h1")},
		{starlark.String("b"), starlark.MakeInt(2)},
	}

	found, rest := scheduler.Hidden(kwargs, spelling.BINDING)

	if found != "h1" {
		t.Fatalf("found %q", found)
	}

	kept := len(rest) == 2 && rest[0][0] == starlark.String("a") &&
		rest[1][0] == starlark.String("b")
	if !kept {
		t.Fatalf("kept %v", rest)
	}
}

// TestHidden_TakesOutEveryPairOfTheName checks no pair of a hidden keyword
// reaches the function called, however many the call carries, and that the
// branch reported is the one the dialect wrote.
//
// Starlark puts the pairs of ** after a call's own, and checks a builtin's
// keywords for nothing: measured 2026-10-03, probe(x = 1, **{"x": 2}) hands a
// builtin two pairs named x, and probe(**{"%builtin": "x"}) one named
// %builtin. So a script can add a hidden pair, and only after the dialect's.
//
// Revisions:
//   - 2026-10-03 23:36: initial creation
func TestHidden_TakesOutEveryPairOfTheName(t *testing.T) {
	into := new(_Lines)
	thread := &starlark.Thread{Name: "spine"}

	settings := &scheduler.Settings{Reporter: into}
	finish := scheduler.Begin(t.Context(), thread, spelling.ENTRY, settings)

	kwargs := []starlark.Tuple{
		{starlark.String(spelling.BUILTIN), starlark.String("if")},
		{starlark.String("name"), starlark.String("ada")},
		{starlark.String(spelling.BUILTIN), starlark.String("written")},
	}

	value, err := starlark.Call(thread, scheduler.CALL, starlark.Tuple{_Greet(t)}, kwargs)

	err = finish(err)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	if value != starlark.String("hello ada") {
		t.Fatalf("got %v", value)
	}

	got := into._All()

	reported := len(got) > 1 && got[1] == "start thread_0 greet if"
	if !reported {
		t.Fatalf("got %v, want greet reported as the dialect's if", got)
	}
}

// _Greet is CALLED's greet, compiled in the runtime's dialect.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
func _Greet(t *testing.T) starlark.Value {
	t.Helper()

	return _Named(t, "greet")
}

// _Named is the global CALLED binds to name, compiled in the runtime's
// dialect.
//
// Revisions:
//   - 2026-10-02 01:20: initial creation, from _Greet's body
func _Named(t *testing.T, name string) starlark.Value {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: "called"},
		"called.star",
		CALLED,
		nil,
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	return globals[name]
}

// _Lines is a reporter that keeps what it was told, one line of text each.
type _Lines struct {
	guard sync.Mutex
	lines []string
}

// Started keeps the start.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
func (l *_Lines) Started(thread string, line *scheduler.Line) {
	l.guard.Lock()
	defer l.guard.Unlock()

	l.lines = append(l.lines, "start "+thread+" "+line.Name+" "+line.Builtin)
}

// Ended keeps the end.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
func (l *_Lines) Ended(thread string, name string, err error) {
	l.guard.Lock()
	defer l.guard.Unlock()

	l.lines = append(l.lines, "end "+thread+" "+name)
}

// _All is everything kept so far.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
func (l *_Lines) _All() []string {
	l.guard.Lock()
	defer l.guard.Unlock()

	return append([]string(nil), l.lines...)
}
