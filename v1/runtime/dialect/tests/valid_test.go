package tests

import (
	"maps"
	"slices"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// TestBound_NamesWhatEachStatementBinds pins the answer for a valid tree, so
// the guards of C87 (nil, typed nil, missing parts) are shown to change
// nothing for the trees a script makes.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBound_NamesWhatEachStatementBinds(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "a def binds its own name only",
			source: "def f():\n    inner = 1\n",
			want:   []string{"f"},
		},
		{
			name:   "an assignment binds a name, a tuple, a list and a paren",
			source: "a = 1\n(b, c) = 2, 3\n[d, e] = 4, 5\n(f) = 6\n",
			want:   []string{"a", "b", "c", "d", "e", "f"},
		},
		{
			name: "a for, a while and an if bind inside their suites",
			source: "for i, j in x:\n    k = 1\n" +
				"while y:\n    m = 2\n" +
				"if z:\n    n = 3\nelse:\n    o = 4\n",
			want: []string{"i", "j", "k", "m", "n", "o"},
		},
		{
			name:   "a load binds what it names",
			source: "load(\"m\", \"p\", q = \"r\")\n",
			want:   []string{"p", "q"},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				file, err := common.OPTIONS.Parse(
					"bound.star",
					test.source,
					0,
				)
				if err != nil {
					t.Fatalf(
						"parse: %v",
						err,
					)
				}

				into := map[string]bool{}

				dialect.Bound(
					file.Stmts,
					into,
				)

				got := slices.Sorted(maps.Keys(into))
				if !slices.Equal(
					got,
					test.want,
				) {
					t.Fatalf(
						"bound %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// TestMatched_ReadsAMatchChain pins Matched, NextCase and Tested for a valid
// match, so the C87 guards change nothing for a script's own tree.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestMatched_ReadsAMatchChain(t *testing.T) {
	var (
		source = "_match = f()\n" +
			"if _match == \"a\":\n    x = 1\n" +
			"elif _match == \"b\":\n    x = 2\n" +
			"elif y:\n    x = 3\n"
		file, err = common.OPTIONS.Parse(
			"match.star",
			source,
			0,
		)
	)
	if err != nil {
		t.Fatalf(
			"parse: %v",
			err,
		)
	}

	assign, chain, ok := dialect.Matched(
		file.Stmts,
		0,
	)
	if !ok || assign == nil || chain == nil {
		t.Fatal("not a match")
	}

	text, tested := dialect.Tested(chain.Cond)
	if !tested || text != "a" {
		t.Fatalf(
			"first arm %q %v, want a",
			text,
			tested,
		)
	}

	next := dialect.NextCase(chain)
	if next == nil {
		t.Fatal("no second arm")
	}

	text, _ = dialect.Tested(next.Cond)
	if text != "b" {
		t.Fatalf(
			"second arm %q, want b",
			text,
		)
	}

	if dialect.NextCase(next) != nil {
		t.Fatal("the else-if of another test is a third arm")
	}

	_, _, ok = dialect.Matched(
		file.Stmts,
		1,
	)
	if ok {
		t.Fatal("the if alone is no match")
	}
}
