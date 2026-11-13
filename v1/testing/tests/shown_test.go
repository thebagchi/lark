// Regression probes for change C47 (decision Q46): str, repr and print refuse
// a value nested past common.DEPTH with an error, where go.starlark.net would
// overflow the Go stack, which nothing can recover.
//
// Revisions:
//   - 2026-11-03 21:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// SHOWN_CHILD names the case a child process runs. Empty in the parent.
	//
	// Revisions:
	//   - 2026-11-03 21:51: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHOWN_CHILD = "LARK_C47_CASE"

	// SHOWN_STACK is the stack limit of the child: 16 MiB. A guarded call stays
	// below it: the walk of common.DEPTH levels needs about 5 MiB. An unguarded
	// walk of SHOWN_DEEP levels needs far more.
	//
	// Revisions:
	//   - 2026-11-03 21:51: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHOWN_STACK = 16 << 20

	// SHOWN_DEEP is the depth the child builds.
	//
	// Revisions:
	//   - 2026-11-03 21:51: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHOWN_DEEP = 400000
)

// SHOWN_LIMIT_SCRIPT wraps a scalar in levels containers of one shape, and
// shows it with one builtin.
//
// Revisions:
//   - 2026-11-03 21:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	SHOWN_LIMIT_SCRIPT = `def main():
    x = 1
    for i in range(%d):
        x = %s
    return len(%s(x))
`
)

// SHOWN_CASES builds one deep value of each container kind, and shows it.
// The key is the case name; the value is the body of main.
//
// Revisions:
//   - 2026-11-03 21:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	SHOWN_CASES = map[string]string{
		"str list":     _Build("[x]") + "    return str(x)",
		"str tuple":    _Build("(x,)") + "    return str(x)",
		"str dict":     _Build(`{"k": x}`) + "    return str(x)",
		"repr list":    _Build("[x]") + "    return repr(x)",
		"repr tuple":   _Build("(x,)") + "    return repr(x)",
		"repr dict":    _Build(`{"k": x}`) + "    return repr(x)",
		"print list":   _Build("[x]") + "    print(x)",
		"print tuple":  _Build("(x,)") + "    print(x)",
		"print dict":   _Build(`{"k": x}`) + "    print(x)",
		"fail list":    _Build("[x]") + "    fail(x)",
		"fail tuple":   _Build("(x,)") + "    fail(x)",
		"fail dict":    _Build(`{"k": x}`) + "    fail(x)",
		"fail second":  _Build("(x,)") + `    fail("a", 1, x)`,
		"print second": _Build("(x,)") + `    print("a", 1, x)`,
	}
)

// _Build is a script line that binds x to a value nested SHOWN_DEEP levels, by
// wrapping it again with step.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Build(step string) string {
	return fmt.Sprintf(
		"def main():\n    x = None\n    for i in range(%d):\n        x = %s\n",
		SHOWN_DEEP,
		step,
	)
}

// TestShown_ADeepValueIsAnErrorNotACrash proves that str, repr and print give
// the error "value nested too deep", after the builtin's name, for a list, a
// tuple and a dict nested 400,000 deep, in a child process whose stack is
// limited to 16 MiB.
//
// Starting condition: the test binary can run itself.
// Success: the child exits 0 and prints the sentence. An unguarded walk
// would end the child with a stack overflow and exit 2.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShown_ADeepValueIsAnErrorNotACrash(t *testing.T) {
	t.Parallel()

	for name := range SHOWN_CASES {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()

				command := exec.Command(
					os.Args[0],
					"-test.run=^TestShown_Child$",
				)

				command.Env = append(
					os.Environ(),
					SHOWN_CHILD+"="+name,
				)

				out, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf(
						"child failed: %v\n%.300s",
						err,
						out,
					)
				}

				builtin, _, _ := strings.Cut(
					name,
					" ",
				)

				var want = builtin + ": value nested too deep"

				if !strings.Contains(
					string(out),
					want,
				) {
					t.Fatalf(
						"child output %.300q, want it to hold %q",
						out,
						want,
					)
				}

				if strings.Count(
					string(out),
					builtin+":",
				) != 1 {
					t.Fatalf(
						"builtin name not said once: %.300q",
						out,
					)
				}
			},
		)
	}
}

// TestShown_Child is the body of the child process. Without SHOWN_CHILD it
// does nothing.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestShown_Child(t *testing.T) {
	name := os.Getenv(SHOWN_CHILD)
	if name == "" {
		t.Skip("runs only as a child of TestShown_ADeepValueIsAnErrorNotACrash")
	}

	debug.SetMaxStack(SHOWN_STACK)

	_, err := _Value(
		t,
		SHOWN_CASES[name]+"\n",
	)

	fmt.Printf(
		"RESULT %v\n",
		err,
	)

	if !errors.Is(
		err,
		trail.ErrDeep,
	) {
		t.Fatalf(
			"got %v, want ErrDeep",
			err,
		)
	}
}

// TestShown_ThroughTheInPackageLimit proves the limit is common.DEPTH: that
// many levels show, one more is refused.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestShown_ThroughTheInPackageLimit(t *testing.T) {
	t.Parallel()

	for _, shape := range []string{"[x]", "(x,)", `{"k": x}`} {
		for _, call := range []string{"str", "repr"} {
			t.Run(
				call+" "+shape,
				func(t *testing.T) {
					t.Parallel()

					body := func(levels int) string {
						return fmt.Sprintf(
							SHOWN_LIMIT_SCRIPT,
							levels,
							shape,
							call,
						)
					}

					// common.DEPTH - 1 wrapping levels plus the scalar: the
					// outermost container is the DEPTH-th.
					var _, err = _Value(
						t,
						body(common.DEPTH),
					)
					if err != nil {
						t.Fatalf(
							"at the limit: %v",
							err,
						)
					}

					_, err = _Value(
						t,
						body(common.DEPTH+1),
					)
					if !errors.Is(
						err,
						trail.ErrDeep,
					) {
						t.Fatalf(
							"past the limit: got %v, want ErrDeep",
							err,
						)
					}
				},
			)
		}
	}
}

// TestShown_NormalValuesAreUnchanged proves the text of str and repr is the
// text go.starlark.net gives, byte for byte, including a value that holds
// itself and one reached by two routes.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShown_NormalValuesAreUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			"int",
			"str(12)",
			`"12"`,
		},
		{
			"string str",
			`str("a'b")`,
			`"a'b"`,
		},
		{
			"string repr",
			`repr("a'b")`,
			`"\"a'b\""`,
		},
		{
			"bytes",
			`str(b"ab")`,
			`"ab"`,
		},
		{
			"list",
			`str([1, "a", None, True])`,
			`"[1, \"a\", None, True]"`,
		},
		{
			"tuple one",
			`repr((1,))`,
			`"(1,)"`,
		},
		{
			"dict",
			`str({"a": [1, (2, 3)]})`,
			`"{\"a\": [1, (2, 3)]}"`,
		},
		{
			"set",
			`str(set([1]))`,
			`"set([1])"`,
		},
		{
			"function",
			`str(main)`,
			`"<function main>"`,
		},
		{
			"cycle list",
			`str(_loop())`,
			`"[1, [...]]"`,
		},
		{
			"cycle dict",
			`str(_loopd())`,
			`"{\"a\": {...}}"`,
		},
		{
			"shared",
			`str(_shared())`,
			`"[[1], [1], {\"x\": [1]}]"`,
		},
		{
			"no argument",
			`str()`,
			``,
		},
		{
			"two arguments",
			`str(1, 2)`,
			``,
		},
		{
			"keyword",
			`str(x = 1)`,
			``,
		},
		{
			"repr no argument",
			`repr()`,
			``,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				got, err := _Value(
					t,
					`
def _loop():
    x = [1]
    x.append(x)
    return x

def _loopd():
    d = {}
    d["a"] = d
    return d

def _shared():
    s = [1]
    return [s, s, {"x": s}]

def main():
    return `+tc.expr+"\n",
				)
				if tc.want == "" {
					if err == nil {
						t.Fatalf(
							"got %s, want an error",
							got,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				if got.String() != tc.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestShown_PrintWritesWhatItAlwaysWrote proves print keeps its separator,
// its strings and bytes unquoted, and its log line.
//
// Revisions:
//   - 2026-11-03 00:31: initial creation (C47)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: gives the run WithLog, WithLogger being dropped (C121, Q340)
func TestShown_PrintWritesWhatItAlwaysWrote(t *testing.T) {
	t.Parallel()

	var lines []string

	_, err := _Value(
		t,
		`
def main():
    print("a", 1, [1, "b"], b"c", sep = "|")
    print()
`,
		runtime.WithLog(&_Catch{
			lines: &lines,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{`a|1|[1, "b"]|c`, ""}
	if strings.Join(
		lines,
		"\n",
	) != strings.Join(
		want,
		"\n",
	) {
		t.Fatalf(
			"got %q, want %q",
			lines,
			want,
		)
	}
}

// TestShown_FailKeepsItsMessage proves fail writes the message it always wrote:
// the prefix "fail: ", strings unquoted, other values as text, and the
// separator. The error is not fatal, since fail ends one thread only.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation (C47, Q61)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShown_FailKeepsItsMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call string
		want string
	}{
		{
			"string",
			`fail("boom")`,
			"fail: boom",
		},
		{
			"mixed",
			`fail("a", 1, [2, "b"], (3,))`,
			`fail: a 1 [2, "b"] (3,)`,
		},
		{
			"sep",
			`fail("a", "b", sep = "|")`,
			"fail: a|b",
		},
		{
			"cycle",
			`fail(_loop())`,
			"fail: [1, [...]]",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _Value(
					t,
					"def _loop():\n    x = [1]\n    x.append(x)\n"+
						"    return x\n\ndef main():\n    "+tc.call+"\n",
				)
				if err == nil {
					t.Fatal("fail returned no error")
				}

				if !strings.Contains(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"got %q, want it to hold %q",
						err,
						tc.want,
					)
				}

				if errors.Is(
					err,
					trail.ErrFatal,
				) {
					t.Fatalf(
						"fail must not end the run: %v",
						err,
					)
				}
			},
		)
	}
}

// _Catch is a writer that keeps the message of each log line WithLog writes to
// it. A line printed empty has no message in the log, and is kept as "".
//
// Revisions:
//   - 2026-11-03 21:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: a writer of log lines, not a slog handler; WithLogger is dropped (C121,
//     Q340)
type _Catch struct {
	lines *[]string
}

// Write keeps the message of the log line p.
//
// Revisions:
//   - 2026-11-10 17:15: initial creation (C121, Q340)
func (c *_Catch) Write(p []byte) (int, error) {
	_, said, found := strings.Cut(
		strings.TrimSuffix(
			string(p),
			"\n",
		),
		" msg=",
	)
	if !found {
		said = ""
	}

	if strings.HasPrefix(
		said,
		`"`,
	) {
		unquoted, err := strconv.Unquote(said)
		if err != nil {
			return 0, err
		}

		said = unquoted
	}

	*c.lines = append(
		*c.lines,
		said,
	)

	return len(p), nil
}
