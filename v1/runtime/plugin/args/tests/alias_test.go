package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/args"
)

// TestArg_AnAliasIsRefusedAtCompile proves arg must be called where it is
// written: binding it to another name, or passing it on, is refused when the
// script compiles, so that every declaration stays visible to the reader of
// the script (Q301). A call with parentheses round the callee is a call, and
// a name of the script's own that hides the builtin is not the builtin.
//
// Revisions:
//   - 2026-11-10 11:48: initial creation (C118, B251, Q301)
//   - 2026-11-10 17:15: uses ErrDeclare, formerly ErrNotDeclaring (C121, Q349)
func TestArg_AnAliasIsRefusedAtCompile(t *testing.T) {
	var tests = []struct {
		name string
		text string
		fail string
	}{
		{
			"bound to a name",
			`
b = arg
h = b("h", 1)

def main():
    return h
`,
			"a.star:1:5",
		},
		{
			"bound to a name in a body",
			`
def main():
    b = arg
    return b
`,
			"a.star:2:9",
		},
		{
			"passed on",
			`
def main():
    return str(arg)
`,
			"a.star:2:16",
		},
		{
			"listed",
			`
fs = [arg]

def main():
    return fs
`,
			"a.star:1:7",
		},
		{
			"parenthesised and aliased",
			`
b = (arg)

def main():
    return b
`,
			"a.star:1:6",
		},
		{
			"parenthesised callee",
			`
h = (arg)("h", 1)

def main():
    return h
`,
			"",
		},
		{
			"called",
			`
h = arg("h", 1)

def main():
    return h
`,
			"",
		},
		{
			"a parameter named arg",
			`
def pick(arg):
    return arg

def main():
    return pick(1)
`,
			"",
		},
		{
			"a local named arg",
			`
def main():
    arg = 1
    return arg
`,
			"",
		},
		{
			"a lambda parameter named arg",
			`
f = lambda arg: arg

def main():
    return f(1)
`,
			"",
		},
		{
			"a keyword named arg",
			`
def pick(arg = 0):
    return arg

def main():
    return pick(arg = 1)
`,
			"",
		},
		{
			"an attribute named arg",
			`
def main():
    return {"arg": 1}.get("arg")
`,
			"",
		},
		{
			"a global of the script's own",
			`
def arg(name, fallback = None):
    return fallback

b = arg

def main():
    return b("h", 1)
`,
			"",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Compile(&common.Source{
					Entry: "a.star",
					Text:  []byte(_Script(tc.text)),
				})
				if tc.fail == "" {
					if err != nil {
						t.Fatalf(
							"got %v, want none",
							err,
						)
					}

					return
				}

				if !errors.Is(
					err,
					args.ErrDeclare,
				) {
					t.Fatalf(
						"got %v, want ErrDeclare",
						err,
					)
				}

				if !strings.HasPrefix(
					err.Error(),
					tc.fail,
				) {
					t.Fatalf(
						"%q does not open with the position %s",
						err,
						tc.fail,
					)
				}
			},
		)
	}
}
