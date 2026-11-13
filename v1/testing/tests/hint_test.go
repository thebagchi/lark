// An error text that has a usual fix names the fix, after the failing text, in
// one style: the text, a space, then the hint in parentheses (change C70).
//
// Revisions:
//   - 2026-11-05 16:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// WAITING is how long TestHints_NoHintForARunStop lets the join start waiting
// before it stops the run.
//
// Revisions:
//   - 2026-11-05 16:36: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	WAITING = 500 * time.Millisecond
)

// _Fails compiles and runs text as probe.star and returns the text of the error
// the compile or the run gives. It fails the test when neither does.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Fails(
	t *testing.T,
	text string,
) error {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: "probe.star",
		Text:  []byte(text),
	})
	if err != nil {
		return err
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err == nil {
		t.Fatalf(
			"script gave no error:\n%s",
			text,
		)
	}

	return err
}

// TestHints_NameTheFix proves each hint: the full text of the failure, the
// sentinel still in the chain, and the hint after the failing text. A case with
// a nil sentinel pins a text that has no sentinel to match.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-05 17:20: the argument case reads token, which the shake
//     would drop otherwise (C44b, Q163)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrClosure and ErrMain, formerly ErrCaptures and ErrNoMain (C121,
//     Q348)
func TestHints_NameTheFix(t *testing.T) {
	cases := []struct {
		name string
		text string
		is   error
		want string
	}{
		{
			name: "a script without main",
			text: "x = 1\n",
			is:   runtime.ErrMain,
			want: "probe.star: no entry point (define def main() with no " +
				"required parameters; a file without it is a module, " +
				"for load)",
		},
		{
			name: "a spawn that captures a local",
			text: "def main():\n    v = 5\n    def w():\n        return v\n" +
				"    join(spawn(w))\n",
			is: runtime.ErrClosure,
			want: "probe.star:5: spawn: w captures v, bound at " +
				"probe.star:2:5: captures a local variable " +
				"(write spawn(lambda: f(x)) to give the thread x as it " +
				"is at the spawn)",
		},
		{
			name: "a join of a cancelled thread",
			text: "def w():\n    sleep(5000)\n\ndef main():\n" +
				"    h = spawn(w)\n    cancel(h)\n    join(h)\n",
			is: runtime.ErrCancelled,
			want: "probe.star:7: w: cancelled: context canceled " +
				"(read a cancelled thread with outcomes())",
		},
		{
			name: "an argument that was not supplied",
			text: "token = arg(\"token\")\n\ndef main():\n    print(token)\n",
			is:   nil,
			want: "initialise probe.star: probe.star:1: token: argument " +
				"not supplied and has no default (supply it as -args " +
				"'{\"token\": ...}', or give arg() a default)",
		},
		{
			name: "a misspelled builtin",
			text: "def main():\n    prnt(\"x\")\n",
			is:   nil,
			want: "probe.star:2:5: undefined: prnt (did you mean print?)",
		},
		{
			name: "a misspelled plugin function",
			text: "def main():\n    spwan(sleep)\n",
			is:   nil,
			want: "probe.star:2:5: undefined: spwan (did you mean spawn?)",
		},
		{
			name: "an assert with a message",
			text: "def main():\n    assert(1 == 2, \"one is not two\")\n",
			is:   runtime.ErrAssert,
			want: "probe.star:2: assertion failed: \"one is not two\"",
		},
		{
			name: "an assert with no message",
			text: "def main():\n    assert(1 == 2)\n",
			is:   runtime.ErrAssert,
			want: "probe.star:2: assertion failed",
		},
		{
			name: "an assert with the message as a keyword",
			text: "def main():\n    assert(msg = \"never\")\n",
			is:   runtime.ErrAssert,
			want: "probe.star:2: assertion failed: \"never\"",
		},
		{
			name: "an assert in a spawned thread",
			text: "def w():\n    assert(False, \"late\")\n\ndef main():\n" +
				"    join(spawn(w))\n",
			is:   runtime.ErrAssert,
			want: "probe.star:2: assertion failed: \"late\"",
		},
		{
			name: "an assert message with a quote and a newline",
			text: "def main():\n    assert(False, \"a\\\"b\\nc\")\n",
			is:   runtime.ErrAssert,
			want: "probe.star:2: assertion failed: \"a\\\"b\\nc\"",
		},
		{
			name: "a name nothing is close to",
			text: "def main():\n    zebra(1)\n",
			is:   nil,
			want: "probe.star:2:5: undefined: zebra",
		},
		{
			name: "a name starlark already suggests keeps its one hint",
			text: "def main():\n    total = 1\n    print(totl)\n",
			is:   nil,
			want: "probe.star:3:11: undefined: totl (did you mean total?)",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := _Fails(
					t,
					tc.text,
				)

				if err.Error() != tc.want {
					t.Errorf(
						"got %q\nwant %q",
						err.Error(),
						tc.want,
					)
				}

				if tc.is != nil && !errors.Is(
					err,
					tc.is,
				) {
					t.Errorf(
						"%v does not match %v",
						err,
						tc.is,
					)
				}

				if strings.Count(
					err.Error(),
					"did you mean",
				) > 1 {
					t.Errorf(
						"more than one suggestion: %v",
						err,
					)
				}
			},
		)
	}
}

// TestHints_NoHintForARunStop proves a join that ends because the whole run
// was stopped does not tell the person to read the thread with outcomes():
// nothing cancelled that thread but the stop.
//
// The join is already waiting when the run is stopped: the thread sleeps for
// five seconds and the test waits 500 ms first. A stop before the join starts
// would not reach the check in core._Ended.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-05 08:25: stops the run while the join waits, so removing the
//     Ambient check in core._Ended fails it
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHints_NoHintForARunStop(t *testing.T) {
	built, err := runtime.Compile(&common.Source{
		Entry: "probe.star",
		Text: []byte("def w():\n    sleep(5000)\n\ndef main():\n" +
			"    join(spawn(w))\n"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run, err := runtime.Start(
		ctx,
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(WAITING)
	stop()

	_, err = run.Wait()
	if !errors.Is(
		err,
		runtime.ErrCancelled,
	) {
		t.Fatalf(
			"got %v, want ErrCancelled",
			err,
		)
	}

	if strings.Contains(
		err.Error(),
		"outcomes()",
	) {
		t.Errorf(
			"a run stop got the cancelled-thread hint: %v",
			err,
		)
	}
}

// TestHints_AssertMessageIsBounded proves a long assert message is cut to the
// Q4 bound, with the marker of the whole length, and still matches runtime.ErrAssert.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70, Q159)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHints_AssertMessageIsBounded(t *testing.T) {
	err := _Fails(
		t,
		"def main():\n    assert(False, \"z\" * 5000000)\n",
	)

	if !errors.Is(
		err,
		runtime.ErrAssert,
	) {
		t.Errorf(
			"%v does not match ErrAssert",
			err,
		)
	}

	if len(err.Error()) > 300 {
		t.Errorf(
			"message of %d bytes is not bounded",
			len(err.Error()),
		)
	}

	if !strings.HasPrefix(
		err.Error(),
		"probe.star:2: assertion failed: \"zzz",
	) {
		t.Errorf(
			"got %.60q",
			err.Error(),
		)
	}
}
