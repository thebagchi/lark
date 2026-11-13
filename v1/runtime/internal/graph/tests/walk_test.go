package tests

import (
	"errors"
	"slices"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	WALK_STILL = "main.star: cycle in the load graph: "
	WALK_ENTRY = "main.star"
	WALK_MAIN  = "def main():\n    pass\n"
)

// TestWalk_RefusalsKeepTheirText proves the dependency walk of graph.Of
// refuses a cycle, a self-load, a missing module and a cycle through the
// entry with the sentinel and the exact text a compile gives.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-05 23:24: the texts of a compile, one for both sides (C60, Q187)
//   - 2026-11-05 23:24: the texts of a compile, one for both sides (C60, Q187)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWalk_RefusalsKeepTheirText(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		files map[string][]byte
		want  error
		text  string
	}{
		{
			name:  "cycle",
			entry: "load(\"a.star\", \"x\")\n" + WALK_MAIN,
			files: map[string][]byte{
				"a.star": []byte("load(\"b.star\", \"y\")\ndef x():\n    pass\n"),
				"b.star": []byte("load(\"a.star\", \"x\")\ndef y():\n    pass\n"),
			},
			want: spelling.ErrCycle,
			text: WALK_STILL + "a.star -> b.star -> a.star",
		},
		{
			name:  "cycle through the entry",
			entry: "load(\"a.star\", \"x\")\n" + WALK_MAIN,
			files: map[string][]byte{
				"a.star": []byte(
					"load(\"main.star\", \"main\")\ndef x():\n    pass\n",
				),
			},
			want: spelling.ErrCycle,
			text: WALK_STILL + "main.star -> a.star -> main.star",
		},
		{
			name:  "self-load",
			entry: "load(\"s.star\", \"y\")\n" + WALK_MAIN,
			files: map[string][]byte{
				"s.star": []byte("load(\"s.star\", \"y\")\n"),
			},
			want: spelling.ErrCycle,
			text: WALK_STILL + "s.star -> s.star",
		},
		{
			name:  "missing module",
			entry: "load(\"gone.star\", \"x\")\n" + WALK_MAIN,
			files: map[string][]byte{},
			text: "main.star: cannot load \"gone.star\" from main.star: " +
				"no such module: gone.star",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := graph.Of(&common.Source{
					Entry: WALK_ENTRY,
					Text:  []byte(test.entry),
					Loader: &_Map{
						files: test.files,
					},
				})
				if err == nil {
					t.Fatal("got nil, want a refusal")
				}

				if test.want != nil && !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}

				if err.Error() != test.text {
					t.Fatalf(
						"got text %q, want %q",
						err.Error(),
						test.text,
					)
				}
			},
		)
	}
}

// TestWalk_DiamondOrder proves a module reached by two routes is declared once,
// and every function precedes the functions of whatever loads it.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWalk_DiamondOrder(t *testing.T) {
	var (
		files = map[string][]byte{
			"l.star":    []byte("load(\"base.star\", \"b\")\ndef l():\n    b()\n"),
			"r.star":    []byte("load(\"base.star\", \"b\")\ndef r():\n    b()\n"),
			"base.star": []byte("def b():\n    pass\n"),
		}
		text = "load(\"l.star\", \"l\")\nload(\"r.star\", \"r\")\n" +
			"def main():\n    l()\n    r()\n"
		want = []string{
			"b",
			"l",
			"r",
		}
	)

	flow, err := graph.Of(&common.Source{
		Entry: WALK_ENTRY,
		Text:  []byte(text),
		Loader: &_Map{
			files: files,
		},
	})
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	var got []string

	for _, fn := range flow.GetFunctions() {
		got = append(
			got,
			fn.GetName(),
		)
	}

	if !slices.Equal(
		got,
		want,
	) {
		t.Fatalf(
			"got order %v, want %v",
			got,
			want,
		)
	}
}

// TestRegression_B042_StatementDropped proves a top-level statement that is no
// def, load or assignment is refused in a loaded module, rather than
// left out of the flow without a message.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, bugs.md B042)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func TestRegression_B042_StatementDropped(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		files map[string][]byte
	}{
		{
			name:  "print in a module",
			entry: "load(\"base.star\", \"b\")\n" + WALK_MAIN,
			files: map[string][]byte{
				"base.star": []byte(
					"print(\"init base\")\n\ndef b():\n    return \"b\"\n",
				),
			},
		},
		{
			name:  "if in a module",
			entry: "load(\"base.star\", \"b\")\n" + WALK_MAIN,
			files: map[string][]byte{
				"base.star": []byte(
					"if True:\n    pass\n\ndef b():\n    return \"b\"\n",
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := graph.Of(&common.Source{
					Entry: WALK_ENTRY,
					Text:  []byte(test.entry),
					Loader: &_Map{
						files: test.files,
					},
				})
				if !errors.Is(
					err,
					graph.ErrCarry,
				) {
					t.Fatalf(
						"got %v, want ErrCarry",
						err,
					)
				}
			},
		)
	}
}

// TestRegression_B043_BuiltinShadowed proves a declaration that another file
// reads as a builtin is refused, and one nobody reads that way is not.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43, bugs.md B043)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegression_B043_BuiltinShadowed(t *testing.T) {
	const (
		MODULE = "def max(a, b):\n    return \"module\"\n\n" +
			"def fm():\n    return max(1, 2)\n"
	)

	tests := []struct {
		name  string
		entry string
		files map[string][]byte
		fail  bool
	}{
		{
			name: "entry reads the builtin",
			entry: "load(\"m.star\", \"fm\")\n\n" +
				"def main():\n    print(fm(), max(1, 2))\n",
			files: map[string][]byte{
				"m.star": []byte(MODULE),
			},
			fail: true,
		},
		{
			name: "another module reads the builtin",
			entry: "load(\"m.star\", \"fm\")\nload(\"u.star\", \"fu\")\n\n" +
				"def main():\n    fm()\n    fu()\n",
			files: map[string][]byte{
				"m.star": []byte(MODULE),
				"u.star": []byte(
					"def fu():\n    return min(1, 2)\n\n" +
						"def fv():\n    return len([1])\n",
				),
			},
			fail: false,
		},
		{
			name: "nobody else reads it",
			entry: "load(\"m.star\", \"fm\")\n\n" +
				"def main():\n    fm()\n",
			files: map[string][]byte{
				"m.star": []byte(MODULE),
			},
			fail: false,
		},
		{
			name: "a local named like a builtin is no read",
			entry: "load(\"m.star\", \"fm\")\n\n" +
				"def main():\n    max = 1\n    print(fm(), max)\n",
			files: map[string][]byte{
				"m.star": []byte(MODULE),
			},
			fail: false,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := graph.Of(&common.Source{
					Entry: WALK_ENTRY,
					Text:  []byte(test.entry),
					Loader: &_Map{
						files: test.files,
					},
				})
				if test.fail {
					if !errors.Is(
						err,
						graph.ErrShadow,
					) {
						t.Fatalf(
							"got %v, want ErrShadow",
							err,
						)
					}

					return
				}

				if err != nil {
					t.Fatalf(
						"got %v, want nil",
						err,
					)
				}
			},
		)
	}
}
