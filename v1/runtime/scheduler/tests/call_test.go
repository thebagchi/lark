package tests

import (
	"fmt"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// CALLED is a script the hidden call is tested against: one function taking a
// keyword, so a test can see that the hidden one did not reach it, and a
// lambda bound to a name, as a loaded module can hold one.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	CALLED = `
def greet(name = "world"):
    return "hello " + name

anonymous = lambda: 1
`
)

// TestCalling_ReportsAScriptFunction checks the hidden call reports a script
// function's call as a line of the thread that made it, with the branch it
// belongs to, and that the function never sees the hidden keyword.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCalling_ReportsAScriptFunction(t *testing.T) {
	var (
		into   = new(_Lines)
		thread = &starlark.Thread{
			Name: "spine",
		}
		settings = &common.Settings{
			Reporter: into,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			settings,
		)
		kwargs = []starlark.Tuple{
			{starlark.String(common.BUILTIN), starlark.String("if")},
			{starlark.String("name"), starlark.String("ada")},
		}
		value, err = starlark.Call(
			thread,
			scheduler.Call(),
			starlark.Tuple{_Greet(t)},
			kwargs,
		)
	)

	err = finish(err)
	if err != nil {
		t.Fatalf(
			"call: %v",
			err,
		)
	}

	if value != starlark.String("hello ada") {
		t.Fatalf(
			"got %v",
			value,
		)
	}

	var (
		want = []string{
			"start thread_0 main ",
			"start thread_0 greet if",
			"end thread_0 greet",
			"end thread_0 main",
		}
		got = into._All()
	)

	if len(got) != len(want) {
		t.Fatalf(
			"got %v, want %v",
			got,
			want,
		)
	}

	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf(
				"got %v, want %v",
				got,
				want,
			)
		}
	}
}

// TestCalling_CallsABuiltinWithoutReporting checks a callee that is not a
// script function is called, and not reported.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCalling_CallsABuiltinWithoutReporting(t *testing.T) {
	var (
		into   = new(_Lines)
		thread = &starlark.Thread{
			Name: "spine",
		}
		settings = &common.Settings{
			Reporter: into,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			settings,
		)
		length     = starlark.Universe["len"]
		args       = starlark.Tuple{length, starlark.String("four")}
		value, err = starlark.Call(
			thread,
			scheduler.Call(),
			args,
			nil,
		)
	)

	err = finish(err)

	if err != nil || value != starlark.MakeInt(len("four")) {
		t.Fatalf(
			"got %v, %v",
			value,
			err,
		)
	}

	if got := into._All(); len(got) != 2 {
		t.Fatalf(
			"want only main's own start and end, got %v",
			got,
		)
	}
}

// TestCalling_CallsALambdaWithoutReporting checks a lambda is called and not
// reported: a loaded name can hold one, and no flow lists it.
//
// Revisions:
//   - 2026-10-02 01:20: initial creation
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCalling_CallsALambdaWithoutReporting(t *testing.T) {
	var (
		into   = new(_Lines)
		thread = &starlark.Thread{
			Name: "spine",
		}
		settings = &common.Settings{
			Reporter: into,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			settings,
		)
		args = starlark.Tuple{_Named(
			t,
			"anonymous",
		)}
		value, err = starlark.Call(
			thread,
			scheduler.Call(),
			args,
			nil,
		)
	)

	err = finish(err)

	if err != nil || value != starlark.MakeInt(1) {
		t.Fatalf(
			"got %v, %v",
			value,
			err,
		)
	}

	if got := into._All(); len(got) != 2 {
		t.Fatalf(
			"want only main's own start and end, got %v",
			got,
		)
	}
}

// TestHidden_TakesTheKeywordOut checks a hidden keyword is read and removed,
// and every other keyword kept in order.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHidden_TakesTheKeywordOut(t *testing.T) {
	var (
		kwargs = []starlark.Tuple{
			{starlark.String("a"), starlark.MakeInt(1)},
			{starlark.String(common.BINDING), starlark.String("h1")},
			{starlark.String("b"), starlark.MakeInt(2)},
		}
		found, rest = scheduler.Hidden(
			kwargs,
			common.BINDING,
		)
	)

	if found != "h1" {
		t.Fatalf(
			"found %q",
			found,
		)
	}

	kept := len(rest) == 2 && rest[0][0] == starlark.String("a") &&
		rest[1][0] == starlark.String("b")
	if !kept {
		t.Fatalf(
			"kept %v",
			rest,
		)
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
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHidden_TakesOutEveryPairOfTheName(t *testing.T) {
	var (
		into   = new(_Lines)
		thread = &starlark.Thread{
			Name: "spine",
		}
		settings = &common.Settings{
			Reporter: into,
		}
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			settings,
		)
		kwargs = []starlark.Tuple{
			{starlark.String(common.BUILTIN), starlark.String("if")},
			{starlark.String("name"), starlark.String("ada")},
			{starlark.String(common.BUILTIN), starlark.String("written")},
		}
		value, err = starlark.Call(
			thread,
			scheduler.Call(),
			starlark.Tuple{_Greet(t)},
			kwargs,
		)
	)

	err = finish(err)
	if err != nil {
		t.Fatalf(
			"call: %v",
			err,
		)
	}

	if value != starlark.String("hello ada") {
		t.Fatalf(
			"got %v",
			value,
		)
	}

	var (
		got      = into._All()
		reported = len(got) > 1 && got[1] == "start thread_0 greet if"
	)

	if !reported {
		t.Fatalf(
			"got %v, want greet reported as the dialect's if",
			got,
		)
	}
}

// _Pair is a keyword pair of a name and a string value.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Pair(
	name string,
	value string,
) starlark.Tuple {
	return starlark.Tuple{
		starlark.String(name),
		starlark.String(value),
	}
}

// TestHidden_WellFormedInputsAreUnchanged pins what Hidden answers for pairs of
// exactly two elements, so a guard for malformed tuples cannot change it.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHidden_WellFormedInputsAreUnchanged(t *testing.T) {
	var (
		a = _Pair(
			"a",
			"1",
		)
		b = _Pair(
			"b",
			"2",
		)
		hid = _Pair(
			"h",
			"x",
		)
		hi2 = _Pair(
			"h",
			"y",
		)
		num = starlark.Tuple{starlark.String("h"), starlark.MakeInt(3)}
	)

	tests := []struct {
		name  string
		input []starlark.Tuple
		found string
		want  string
	}{
		{
			"nil",
			nil,
			"",
			"[]",
		},
		{
			"absent",
			[]starlark.Tuple{a, b},
			"",
			"[(\"a\", \"1\") (\"b\", \"2\")]",
		},
		{
			"only",
			[]starlark.Tuple{hid},
			"x",
			"[]",
		},
		{
			"first",
			[]starlark.Tuple{hid, a, b},
			"x",
			"[(\"a\", \"1\") (\"b\", \"2\")]",
		},
		{
			"last",
			[]starlark.Tuple{a, b, hid},
			"x",
			"[(\"a\", \"1\") (\"b\", \"2\")]",
		},
		{
			"between",
			[]starlark.Tuple{a, hid, b},
			"x",
			"[(\"a\", \"1\") (\"b\", \"2\")]",
		},
		{
			"twice",
			[]starlark.Tuple{hid, a, hi2},
			"x",
			"[(\"a\", \"1\")]",
		},
		{
			"non-string value",
			[]starlark.Tuple{a, num},
			"",
			"[(\"a\", \"1\")]",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				found, rest := scheduler.Hidden(
					tc.input,
					"h",
				)

				if found != tc.found {
					t.Fatalf(
						"found %q, want %q",
						found,
						tc.found,
					)
				}

				if got := fmt.Sprint(rest); got != tc.want {
					t.Fatalf(
						"rest %s, want %s",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestHidden_MalformedTupleDoesNotMatch checks a tuple that is not exactly two
// elements never matches the name, never panics, and stays in the rest, in
// order. The interpreter always passes pairs; only a Go caller can pass these.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation, for bug B056 (decision Q55)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHidden_MalformedTupleDoesNotMatch(t *testing.T) {
	var (
		a = _Pair(
			"a",
			"1",
		)
		hid = _Pair(
			"h",
			"x",
		)
		empty = starlark.Tuple{}
		one   = starlark.Tuple{starlark.String("h")}
		three = starlark.Tuple{
			starlark.String("h"),
			starlark.String("z"),
			starlark.String("extra"),
		}
		badKey = starlark.Tuple{starlark.MakeInt(1), starlark.String("h")}
	)

	tests := []struct {
		name  string
		input []starlark.Tuple
		found string
		want  []starlark.Tuple
	}{
		{
			"empty",
			[]starlark.Tuple{empty},
			"",
			[]starlark.Tuple{empty},
		},
		{
			"one element",
			[]starlark.Tuple{one},
			"",
			[]starlark.Tuple{one},
		},
		{
			"three elements",
			[]starlark.Tuple{three},
			"",
			[]starlark.Tuple{three},
		},
		{
			"non-string key",
			[]starlark.Tuple{badKey},
			"",
			[]starlark.Tuple{badKey},
		},
		{
			"malformed before",
			[]starlark.Tuple{one, hid, a},
			"x",
			[]starlark.Tuple{one, a},
		},
		{
			"malformed after",
			[]starlark.Tuple{a, hid, one},
			"x",
			[]starlark.Tuple{a, one},
		},
		{
			"malformed between",
			[]starlark.Tuple{hid, empty, a},
			"x",
			[]starlark.Tuple{empty, a},
		},
		{
			"name twice, one malformed",
			[]starlark.Tuple{one, hid, three},
			"x",
			[]starlark.Tuple{one, three},
		},
		{
			"name twice, malformed second",
			[]starlark.Tuple{hid, a, one},
			"x",
			[]starlark.Tuple{a, one},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				found, rest := scheduler.Hidden(
					tc.input,
					"h",
				)

				if found != tc.found {
					t.Fatalf(
						"found %q, want %q",
						found,
						tc.found,
					)
				}

				if fmt.Sprint(rest) != fmt.Sprint(tc.want) {
					t.Fatalf(
						"rest %v, want %v",
						rest,
						tc.want,
					)
				}
			},
		)
	}
}

// TestHidden_SingleElementTupleDoesNotPanic is the breaker's case for bug B056:
// one pair with only the key.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHidden_SingleElementTupleDoesNotPanic(t *testing.T) {
	found, rest := scheduler.Hidden(
		[]starlark.Tuple{{starlark.String("target")}},
		"target",
	)

	if found != "" || len(rest) != 1 {
		t.Fatalf(
			"found %q rest %v",
			found,
			rest,
		)
	}
}

// _Greet is CALLED's greet, compiled in the runtime's dialect.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Greet(t *testing.T) starlark.Value {
	t.Helper()

	return _Named(
		t,
		"greet",
	)
}

// _Named is the global CALLED binds to name, compiled in the runtime's
// dialect.
//
// Revisions:
//   - 2026-10-02 01:20: initial creation, from _Greet's body
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Named(
	t *testing.T,
	name string,
) starlark.Value {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: "called",
		},
		"called.star",
		CALLED,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	return globals[name]
}

// _Lines is a reporter that keeps what it was told, one line of text each.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Lines struct {
	guard sync.Mutex
	lines []string
}

// Started keeps the start.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (l *_Lines) Started(
	thread string,
	line *common.Line,
) {
	l.guard.Lock()
	defer l.guard.Unlock()

	l.lines = append(
		l.lines,
		"start "+thread+" "+line.Name+" "+line.Builtin,
	)
}

// Ended keeps the end.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (l *_Lines) Ended(
	thread string,
	name string,
	err error,
) {
	l.guard.Lock()
	defer l.guard.Unlock()

	l.lines = append(
		l.lines,
		"end "+thread+" "+name,
	)
}

// _All is everything kept so far.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (l *_Lines) _All() []string {
	l.guard.Lock()
	defer l.guard.Unlock()

	return append(
		[]string(nil),
		l.lines...,
	)
}
