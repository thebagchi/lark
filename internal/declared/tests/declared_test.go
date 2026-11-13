package tests

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/thebagchi/lark/internal/declared"
)

// TestOf_ReadsEveryTopLevelArgCall proves the names are those of every
// arg() call outside a body, sorted and once each, and that a name the walk
// cannot know is refused (Q193).
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 11:46: a parenthesised callee, and a bytes name (C118, B251, B252)
func TestOf_ReadsEveryTopLevelArgCall(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want []string
		fail error
	}{
		{
			"assigned",
			"a = arg(\"x\")\n",
			[]string{"x"},
			nil,
		},
		{
			"printed",
			"print(arg(\"x\", 1))\n",
			[]string{"x"},
			nil,
		},
		{
			"listed and passed",
			"f(arg(\"b\"), [arg(\"a\")])\n",
			[]string{"a", "b"},
			nil,
		},
		{
			"twice",
			"a = arg(\"x\")\nb = arg(\"x\")\n",
			[]string{"x"},
			nil,
		},
		{
			"in a def",
			"def f():\n    return arg(\"x\")\n",
			nil,
			nil,
		},
		{
			"in a lambda",
			"f = lambda: arg(\"x\")\n",
			nil,
			nil,
		},
		{
			"computed name",
			"a = arg(\"x\" + \"y\")\n",
			nil,
			errors.New("not a string literal"),
		},
		{
			"parenthesised callee",
			"h = (arg)(\"h\", 1)\n",
			[]string{"h"},
			nil,
		},
		{
			"doubly parenthesised callee",
			"h = ((arg))(\"h\")\n",
			[]string{"h"},
			nil,
		},
		{
			"bytes name",
			"h = arg(b\"x\", 1)\n",
			nil,
			errors.New("not a string literal"),
		},
		{
			"bytes name in a parenthesised callee",
			"h = (arg)(b\"x\")\n",
			nil,
			errors.New("not a string literal"),
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got, err := declared.Of(
					"d.star",
					[]byte(tc.text),
				)
				if (tc.fail == nil) != (err == nil) || (err != nil &&
					!strings.Contains(
						err.Error(),
						tc.fail.Error(),
					)) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.fail,
					)
				}

				if !slices.Equal(
					got,
					tc.want,
				) {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestOf_ReadsNothingFromAScriptThatBindsArg proves a script that binds the
// name arg itself declares nothing: its calls of arg are calls of its own
// function, the compile skips the plugin that supplies a shadowed name, and a
// run of it takes the -args keys as declared by nobody (B261).
//
// Every way a module binds a name counts: a def, an assignment, a load, a for
// at the top level, and a binding inside an if. A parameter or a local of a
// def does not bind the name for the module, and a def holds no declaration
// anyway.
//
// Revisions:
//   - 2026-11-11 00:31: initial creation (C122, B261)
func TestOf_ReadsNothingFromAScriptThatBindsArg(t *testing.T) {
	var tests = []struct {
		name string
		text string
		want []string
		fail error
	}{
		{
			"a def named arg",
			`
def arg(name):
    return name

x = arg("x")
`,
			nil,
			nil,
		},
		{
			"a def named arg, called with a name it cannot know",
			`
def arg(name):
    return name

x = arg("x" + "y")
`,
			nil,
			nil,
		},
		{
			"a def named arg, after the call",
			`
x = arg("x")

def arg(name):
    return name
`,
			nil,
			nil,
		},
		{
			"an assignment of arg",
			`
arg = print
arg("x")
`,
			nil,
			nil,
		},
		{
			"a load of arg",
			`
load("m.star", "arg")

x = arg("x")
`,
			nil,
			nil,
		},
		{
			"a load of arg under another name",
			`
load("m.star", arg = "other")

x = arg("x")
`,
			nil,
			nil,
		},
		{
			"a for at the top level",
			`
for arg in [print]:
    arg("x")

y = arg("y")
`,
			nil,
			nil,
		},
		{
			"a binding inside an if",
			`
if True:
    def arg(name):
        return name

x = arg("x")
`,
			nil,
			nil,
		},
		{
			"a parameter named arg does not bind it for the module",
			`
def f(arg):
    return arg

x = arg("x")
`,
			[]string{"x"},
			nil,
		},
		{
			"a local named arg in a def does not bind it for the module",
			`
def f():
    arg = 1
    return arg

x = arg("x")
`,
			[]string{"x"},
			nil,
		},
		{
			"another name does not",
			`
def argument(name):
    return name

x = arg("x")
`,
			[]string{"x"},
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got, err := declared.Of(
					"d.star",
					[]byte(strings.TrimPrefix(
						tc.text,
						"\n",
					)),
				)
				if (tc.fail == nil) != (err == nil) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.fail,
					)
				}

				if !slices.Equal(
					got,
					tc.want,
				) {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}
