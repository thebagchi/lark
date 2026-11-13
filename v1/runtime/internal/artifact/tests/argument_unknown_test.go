package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// UNKNOWN_SCRIPT declares "host" and "port", which main reads, under the
	// names server and number: the name a caller supplies is the string.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	UNKNOWN_SCRIPT = `
server = arg("host", "localhost")
number = arg("port", 8080)

def main():
    return server + ":" + str(number)
`
)

// _Starting is UNKNOWN_SCRIPT, compiled, and the same loaded back from the
// bundle Save wrote, so a test runs both.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Starting(t *testing.T) map[string]common.Artifact {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: "unknown.star",
		Text:  []byte(UNKNOWN_SCRIPT),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	loaded, err := runtime.Load(saved)
	if err != nil {
		t.Fatalf(
			"load: %v",
			err,
		)
	}

	return map[string]common.Artifact{
		"compiled": built,
		"bundle":   loaded,
	}
}

// TestWithArgs_RefusesAKeyNoArgDeclares proves a run handed a key no arg()
// declares is refused with spelling.ErrArity, naming the key (Q170, Q185), and a run
// handed the declared keys is not (Q4: the key is quoted).
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func TestWithArgs_RefusesAKeyNoArgDeclares(t *testing.T) {
	var tests = []struct {
		name string
		args map[string]any
		want string
		fail string
		hint string
	}{
		{
			name: "declared key",
			args: map[string]any{"host": "db"},
			want: "db:8080",
		},
		{
			name: "every declared key",
			args: map[string]any{"host": "db", "port": 1},
			want: "db:1",
		},
		{
			name: "no key",
			args: map[string]any{},
			want: "localhost:8080",
		},
		{
			name: "unknown key",
			args: map[string]any{"colour": "red"},
			fail: `"colour"`,
		},
		{
			name: "name of the binding, not of the argument",
			args: map[string]any{"server": "db"},
			fail: `"server"`,
		},
		{
			name: "declared and unknown",
			args: map[string]any{"host": "db", "colour": "red"},
			fail: `"colour"`,
		},
		{
			name: "near miss",
			args: map[string]any{"hosts": "db"},
			fail: `"hosts"`,
			hint: `(did you mean "host"?)`,
		},
		{
			name: "far miss has no hint",
			args: map[string]any{"zebra": 1},
			fail: `"zebra"`,
		},
	}

	for mode, art := range _Starting(t) {
		for _, tc := range tests {
			t.Run(
				mode+"/"+tc.name,
				func(t *testing.T) {
					got, err := runtime.Eval(
						t.Context(),
						art,
						artifact.WithArgs(tc.args),
					)

					if tc.fail == "" {
						if err != nil {
							t.Fatalf(
								"got %v, want none",
								err,
							)
						}

						if got.Value.String() != `"`+tc.want+`"` {
							t.Fatalf(
								"got %s, want %q",
								got.Value,
								tc.want,
							)
						}

						return
					}

					if !errors.Is(
						err,
						spelling.ErrArity,
					) {
						t.Fatalf(
							"got %v, want ErrArity",
							err,
						)
					}

					if !strings.Contains(
						err.Error(),
						tc.fail,
					) {
						t.Fatalf(
							"%q does not name %s",
							err,
							tc.fail,
						)
					}

					if tc.hint != "" && !strings.Contains(
						err.Error(),
						tc.hint,
					) {
						t.Fatalf(
							"%q lacks the hint %s",
							err,
							tc.hint,
						)
					}

					if tc.hint == "" && strings.Contains(
						err.Error(),
						"did you mean",
					) {
						t.Fatalf(
							"%q has a hint, want none",
							err,
						)
					}
				},
			)
		}
	}
}

// TestWithArgs_BoundsAnUnknownKey proves a 5000-byte unknown key is cut in
// the error (Q4).
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func TestWithArgs_BoundsAnUnknownKey(t *testing.T) {
	var long = strings.Repeat(
		"k",
		5000,
	)

	for mode, art := range _Starting(t) {
		t.Run(
			mode,
			func(t *testing.T) {
				_, err := runtime.Eval(
					t.Context(),
					art,
					artifact.WithArgs(map[string]any{long: 1}),
				)
				if !errors.Is(
					err,
					spelling.ErrArity,
				) {
					t.Fatalf(
						"got %v, want ErrArity",
						err,
					)
				}

				if len(err.Error()) > 1000 {
					t.Fatalf(
						"error is %d bytes, want it bounded",
						len(err.Error()),
					)
				}
			},
		)
	}
}

const (
	// INLINE_SCRIPT declares at its top level without assigning: in a call, in
	// a list, and as the argument of a function. late is a call in a body.
	//
	// Revisions:
	//   - 2026-11-05 23:04: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	INLINE_SCRIPT = `
print(arg("greeting", "hi"))
names = [arg("first", "a"), arg("second", "b")]

def pick(x):
    return x

pick(arg("third", "c"))

def main():
    return names[0]
`
	BODY_SCRIPT = `
def main():
    return "x"

def late():
    return arg("late", 1)
`

	// PAREN_SCRIPT writes the callee of its declaration in parentheses: a run
	// reads it as arg(), so the name it states is declared (Q301).
	//
	// Revisions:
	//   - 2026-11-10 11:46: initial creation (C118, B251, Q301)
	PAREN_SCRIPT = `
limit = (arg)("limit", 3)

def main():
    return limit
`
)

// TestWithArgs_CountsEveryTopLevelArgCall proves a key is declared by any
// arg() call at the top level, assigned or not (Q193), that an arg() in a body
// declares nothing, and that a misspelled inline key gets the hint (Q158).
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71, Q193)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 11:46: a parenthesised callee declares (C118, B251, Q301)
func TestWithArgs_CountsEveryTopLevelArgCall(t *testing.T) {
	var tests = []struct {
		name   string
		script string
		key    string
		fail   bool
		hint   string
	}{
		{
			"print",
			INLINE_SCRIPT,
			"greeting",
			false,
			"",
		},
		{
			"list",
			INLINE_SCRIPT,
			"second",
			false,
			"",
		},
		{
			"argument of a call",
			INLINE_SCRIPT,
			"third",
			false,
			"",
		},
		{
			"misspelled inline",
			INLINE_SCRIPT,
			"greetings",
			true,
			`(did you mean "greeting"?)`,
		},
		{
			"in a body",
			BODY_SCRIPT,
			"late",
			true,
			"",
		},
		{
			"parenthesised callee",
			PAREN_SCRIPT,
			"limit",
			false,
			"",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "inline.star",
						Text:  []byte(tc.script),
					},
					artifact.WithArgs(map[string]any{tc.key: "z"}),
				)
				if !tc.fail {
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
					spelling.ErrArity,
				) {
					t.Fatalf(
						"got %v, want ErrArity",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					tc.hint,
				) {
					t.Fatalf(
						"%q lacks %q",
						err,
						tc.hint,
					)
				}
			},
		)
	}
}
