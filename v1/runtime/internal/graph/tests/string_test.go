package tests

import (
	"slices"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// STRING_TRIPLE_SINGLE is a script with a triple single-quoted string over lines, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_TRIPLE_SINGLE = `
def main():
    y = '''a
b
    c

d'''
    return y
`

	// STRING_TRIPLE_DOUBLE is a script with a triple double-quoted string, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_TRIPLE_DOUBLE = `
def main():
    y = """one
  two
three"""
    return y
`

	// STRING_RAW is a script with raw strings, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_RAW = `
def main():
    y = r'''a\
  b\n
c'''
    z = r"""p
 q"""
    return y + z
`

	// STRING_BYTES is a script with a bytes literal, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_BYTES = `
def main():
    y = b'''a
  b'''
    return y
`

	// STRING_BLANKS is a script with blank and whitespace-only lines, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_BLANKS = `
def main():
    y = '''a

   
	
b'''
    return y
`

	// STRING_CONTINUED is a script with a backslash continuation inside a string, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_CONTINUED = `
def main():
    y = 'a\
  b\
c'
    return y
`

	// STRING_CODE_AFTER is a script with code after the closing quotes, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_CODE_AFTER = `
def main():
    y = f('''a
  b''', 1,
      2)
    return y
`

	// STRING_NESTED is a script with a string in an if and a for, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_NESTED = `
def main():
    if True:
        for i in [1]:
            y = '''a
b
        c'''
            return y
    return 0
`

	// STRING_HELPER is a script with a string in a helper function, in a helper.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_HELPER = `
def helper():
    y = '''a
b
    c'''
    return y

def main():
    return helper()
`

	// STRING_INIT is a script with a string in an init statement, in the init.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_INIT = `
[ '''a
b
    c''' ]

def main():
    return 1
`

	// STRING_TWICE is a script with two strings in one statement, in main.
	//
	// Revisions:
	//   - 2026-11-12 18:19: initial creation (C128, B282)
	STRING_TWICE = `
def main():
    y = ['''a
b''', """c
 d"""]
    return y
`
)

// TestOf_KeepsEveryStringValue proves a string literal of the script has the
// same value after Of and Emit, and after a second Of and Emit: a body is
// written under a def, and the indent of the def goes on no line inside a string
// (B282). The values are read with the parser, so a raw string, a bytes literal
// and a backslash continuation are compared as the script means them.
//
// Revisions:
//   - 2026-11-12 18:19: initial creation (C128, B282)
func TestOf_KeepsEveryStringValue(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"a triple single-quoted string over lines",
			STRING_TRIPLE_SINGLE,
		},
		{
			"a triple double-quoted string",
			STRING_TRIPLE_DOUBLE,
		},
		{
			"raw strings",
			STRING_RAW,
		},
		{
			"a bytes literal",
			STRING_BYTES,
		},
		{
			"blank and whitespace-only lines",
			STRING_BLANKS,
		},
		{
			"a backslash continuation inside a string",
			STRING_CONTINUED,
		},
		{
			"code after the closing quotes",
			STRING_CODE_AFTER,
		},
		{
			"a string in an if and a for",
			STRING_NESTED,
		},
		{
			"a string in a helper function",
			STRING_HELPER,
		},
		{
			"a string in an init statement",
			STRING_INIT,
		},
		{
			"two strings in one statement",
			STRING_TWICE,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					want = Strings(
						t,
						tc.text,
					)
					first = _Written(
						t,
						tc.text,
					)
					second = _Written(
						t,
						first,
					)
				)

				for _, got := range []string{first, second} {
					if !slices.Equal(
						Strings(
							t,
							got,
						),
						want,
					) {
						t.Fatalf(
							"strings %q, want %q, in %q",
							Strings(
								t,
								got,
							),
							want,
							got,
						)
					}
				}
			},
		)
	}
}

// Strings is the value of every string and bytes literal in a script, sorted,
// as the parser reads them.
//
// Revisions:
//   - 2026-11-12 18:19: initial creation (C128, B282)
func Strings(
	t *testing.T,
	text string,
) []string {
	t.Helper()

	file, err := common.OPTIONS.Parse(
		"strings.star",
		text,
		0,
	)
	if err != nil {
		t.Fatalf(
			"parse %q: %v",
			text,
			err,
		)
	}

	var found []string

	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			lit, ok := node.(*syntax.Literal)
			if !ok {
				return true
			}

			value, ok := lit.Value.(string)
			if ok {
				found = append(
					found,
					value,
				)
			}

			return true
		},
	)

	slices.Sort(found)

	return found
}
