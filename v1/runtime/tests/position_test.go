package tests

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// ENTRY_NAME is the file name every script of these tests is compiled under, and
// so the file a position names.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	ENTRY_NAME = "t.star"
)

// _Modules is a Loader holding one module, mod.star, whose top level fails.
//
// Revisions:
//   - 2026-11-04 18:40: initial creation (C69)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Modules struct{}

// Resolve is target as it is.
//
// Revisions:
//   - 2026-11-04 18:40: initial creation (C69)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Modules) Resolve(
	_ string,
	target string,
) (string, error) {
	return target, nil
}

// Load is the text of mod.star, or an error for any other name.
//
// Revisions:
//   - 2026-11-04 18:40: initial creation (C69)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (m *_Modules) Load(name string) ([]byte, error) {
	if name != "mod.star" {
		return nil, fmt.Errorf(
			"no module %s",
			name,
		)
	}

	return []byte("x = [1][9]\n"), nil
}

// _Failed compiles text as ENTRY_NAME, runs it, and returns the error the run
// gave, failing the test when the run did not fail.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-04 18:40: runs with a loader holding a failing module (C69, fix round 2)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Failed(
	t *testing.T,
	text string,
) error {
	t.Helper()

	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry:  ENTRY_NAME,
			Text:   []byte(text),
			Loader: new(_Modules),
		},
	)
	if err == nil {
		t.Fatalf(
			"the run of %q passed, want a failure",
			text,
		)
	}

	return err
}

// TestPosition_RunErrorsNameWhereTheyArose proves a run-time error says, in its
// one line, the file and line of the innermost Starlark frame: the
// place the error arose, not the place the call began (decision Q80).
//
// Each case is one kind of error. The line is that of the call, or of the
// operator, as the interpreter reports it. There is no column (decision Q89).
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-04 18:40: module-level and loaded-module cases: wrapper words come
//     before the position (C69, fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_RunErrorsNameWhereTheyArose(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			"fail",
			"def main():\n    fail(\"boom\")\n",
			"t.star:2: fail: boom",
		},
		{
			"fail three calls deep names the innermost frame",
			"def f():\n    fail(\"boom\")\n\n" +
				"def g():\n    f()\n\ndef main():\n    g()\n",
			"t.star:2: fail: boom",
		},
		{
			"assert with a message",
			"def main():\n    assert(False, \"nope\")\n",
			"t.star:2: assertion failed: \"nope\"",
		},
		{
			"assert with no message",
			"def main():\n    assert(1 == 2)\n",
			"t.star:2: assertion failed",
		},
		{
			"assert by keyword",
			"def main():\n    assert(msg = \"never\")\n",
			"t.star:2: assertion failed: \"never\"",
		},
		{
			"assert refused for a lone string",
			"def main():\n    assert(\"text\")\n",
			"t.star:2: assert got only the message",
		},
		{
			"builtin error",
			"def main():\n    sleep(\"a\")\n",
			"t.star:2: sleep: ",
		},
		{
			"builtin argument count",
			"def main():\n    sleep()\n",
			"t.star:2: sleep: got 0 arguments",
		},
		{
			"language error",
			"def main():\n    x = 1 + \"a\"\n",
			"t.star:2: unknown binary op: int + string",
		},
		{
			"fail at module level, wrapper words first",
			"fail(\"top\")\n\ndef main():\n    pass\n",
			"initialise t.star: t.star:1: fail: top",
		},
		{
			"error raised by a module a script loads",
			"load(\"mod.star\", \"x\")\n\ndef main():\n    print(x)\n",
			"initialise t.star: mod.star:1: ",
		},
		{
			"error in a function the module level calls",
			"def f():\n    fail(\"early\")\n\nf()\n\ndef main():\n    pass\n",
			"initialise t.star: t.star:2: fail: early",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := _Failed(
					t,
					tc.text,
				)

				if !strings.HasPrefix(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"got %q, want it to begin %q",
						err.Error(),
						tc.want,
					)
				}
			},
		)
	}
}

// TestPosition_SpawnedThreadsNameTheirOwnFrame proves a failure in a spawned
// function names the line in that function, whether the script joins it, reads
// it with outcomes, or lets the run end on it.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_SpawnedThreadsNameTheirOwnFrame(t *testing.T) {
	const WORKER = "def w():\n    fail(\"inside\")\n\n"

	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			"join raises it",
			WORKER + "def main():\n    join(spawn(w))\n",
			"w: t.star:2: fail: inside",
		},
		{
			"an assert in an unjoined thread ends the run",
			"def w():\n    assert(False, \"late\")\n\n" +
				"def main():\n    h = spawn(w)\n    sleep(1000)\n",
			"t.star:2: assertion failed: \"late\"",
		},
		{
			"an error three calls deep in a thread",
			"def f():\n    x = 1 + \"a\"\n\ndef g():\n    f()\n\n" +
				"def w():\n    g()\n\ndef main():\n    join(spawn(w))\n",
			"w: t.star:2: unknown binary op: int + string",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := _Failed(
					t,
					tc.text,
				)

				if !strings.HasPrefix(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"got %q, want it to begin %q",
						err.Error(),
						tc.want,
					)
				}
			},
		)
	}
}

// TestPosition_OutcomesRecordCarriesIt proves the error text of an outcomes
// record is the same one line, position first, so a script that prints it
// shows where the thread failed.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_OutcomesRecordCarriesIt(t *testing.T) {
	value, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: ENTRY_NAME,
			Text: []byte(
				"def w():\n    fail(\"inside\")\n\n" +
					"def main():\n" +
					"    return outcomes(spawn(w))[0][\"error\"]\n",
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	text, ok := starlark.AsString(value)
	if !ok {
		t.Fatalf(
			"got %v, want a string",
			value,
		)
	}

	const WANT = "t.star:2: fail: inside"
	if text != WANT {
		t.Fatalf(
			"got %q, want %q",
			text,
			WANT,
		)
	}
}

// TestPosition_WrappersKeepIt proves retry, timeout and repeat keep the
// position: each puts its own words first, as it did, and the place the
// function failed follows them.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_WrappersKeepIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			"retry that gives up",
			"def a():\n    assert(False, \"no\")\n\ndef main():\n    retry(2, a)\n",
			"retry(2, a) gave up after 2 attempts: " +
				"t.star:2: assertion failed: \"no\"",
		},
		{
			"timeout around a failure",
			"def a():\n    fail(\"no\")\n\ndef main():\n    timeout(5000, a)\n",
			"timeout(5000, a): t.star:2: fail: no",
		},
		{
			"repeat around a failure",
			"def a():\n    fail(\"no\")\n\ndef main():\n    repeat(2, a)\n",
			"repeat(2, a) attempt 1: t.star:2: fail: no",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := _Failed(
					t,
					tc.text,
				)

				if err.Error() != tc.want {
					t.Fatalf(
						"got %q, want %q",
						err.Error(),
						tc.want,
					)
				}
			},
		)
	}
}

// TestPosition_StillMatchesTheSentinel proves the position changes the text
// and nothing else: errors.Is reaches the sentinel and errors.As still finds
// the interpreter's error, so a host written before C69 behaves as it did.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_StillMatchesTheSentinel(t *testing.T) {
	err := _Failed(
		t,
		"def main():\n    assert(False, \"x\")\n",
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

	err = _Failed(
		t,
		"def main():\n    fail(\"x\")\n",
	)

	var evaluated *starlark.EvalError
	if !errors.As(
		err,
		&evaluated,
	) {
		t.Fatalf(
			"got %T, want an *starlark.EvalError in the chain",
			err,
		)
	}
}

// TestPosition_LongFileNameIsCut proves a file name longer than
// common.QUOTE_LIMIT bytes is cut in the position, as C21 cuts a path in an
// error (decision Q91), and a name of that length or less prints whole.
//
// Revisions:
//   - 2026-11-04 18:12: initial creation (C69)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_LongFileNameIsCut(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry string
		whole bool
	}{
		{
			"exactly the limit prints whole",
			strings.Repeat(
				"d",
				common.QUOTE_LIMIT-len(".star"),
			) + ".star",
			true,
		},
		{
			"a long name is cut",
			strings.Repeat(
				"d",
				5000,
			) + ".star",
			false,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Run(
					t.Context(),
					&common.Source{
						Entry: tc.entry,
						Text:  []byte("def main():\n    fail(\"boom\")\n"),
					},
				)
				if err == nil {
					t.Fatal("the run passed, want a failure")
				}

				text := err.Error()

				if tc.whole != strings.HasPrefix(
					text,
					tc.entry+":2: fail: boom",
				) {
					t.Fatalf(
						"got %.150q, whole name is %v",
						text,
						tc.whole,
					)
				}

				if len(text) > 2*common.QUOTE_LIMIT+len(": fail: boom")+40 {
					t.Fatalf(
						"error is %d bytes, want the name cut",
						len(text),
					)
				}

				if !strings.HasSuffix(
					text,
					":2: fail: boom",
				) {
					t.Fatalf(
						"got %.150q, want the line and text kept",
						text,
					)
				}
			},
		)
	}
}

// TestPosition_FlowNamesTheGeneratedScript proves a failure in a script that a
// flow made names a line of that generated script: the line Emit gives for the
// failing statement. The command names the script by the flow's path with
// ".star" on the end, and -translate prints it.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file and line, with no column (Q89)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPosition_FlowNamesTheGeneratedScript(t *testing.T) {
	var (
		flow = _Flow(
			t,
			`{
			"functions": [{
				"name": "boom",
				"params": ["x"],
				"body": "fail(\"flow boom\")"
			}],
			"text": "return boom(21)"
		}`,
		)
		generated, err = runtime.Emit(flow)
		line           = 0
	)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	for index, text := range strings.Split(
		string(generated),
		"\n",
	) {
		if strings.Contains(
			text,
			"fail(",
		) {
			line = index + 1
		}
	}

	if line == 0 {
		t.Fatalf(
			"no fail( in the generated script:\n%s",
			generated,
		)
	}

	_, err = runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "run.json.star",
			Text:  generated,
		},
	)
	if err == nil {
		t.Fatal("the flow passed, want a failure")
	}

	want := fmt.Sprintf(
		"run.json.star:%d: ",
		line,
	)
	if !strings.HasPrefix(
		err.Error(),
		want,
	) || !strings.Contains(
		err.Error(),
		"fail: flow boom",
	) {
		t.Fatalf(
			"got %q, want it to begin %q\n%s",
			err.Error(),
			want,
			generated,
		)
	}
}
