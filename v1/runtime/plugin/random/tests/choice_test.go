package tests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// SEEDED is a script that seeds the source and draws twelve choices from a
// sequence. AFTER draws a choice and then an int, to show that a choice
// consumes exactly one draw.
//
// Revisions:
//   - 2026-11-02 12:01: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	SEEDED = "random.seed(42); return [random.choice(%s) for i in range(12)]"
	AFTER  = "random.seed(1); a = random.choice(%s); return [a, random.int(1, 100)]"
	SINGLE = "return random.choice(%s)"
	CALLS  = 100
)

// PLACE is the position a run-time error begins with: file, line and column.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	PLACE = regexp.MustCompile(`^[^:\s]+:\d+: `)
)

// TestChoice_KeepsTheSequenceOfASeed pins what random.choice gives after
// random.seed, for each kind of iterable. The want values were captured from
// the code that copied the iterable first. A seed names a sequence, so the
// draw must stay one IntN of the length, and the index must be the position
// the iterable yields.
//
// Revisions:
//   - 2026-11-02 11:30: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestChoice_KeepsTheSequenceOfASeed(t *testing.T) {
	cases := []struct {
		name     string
		script   string
		sequence string
		want     string
	}{
		{
			"list",
			SEEDED,
			"[10, 20, 30, 40, 50]",
			"[40, 20, 40, 30, 50, 20, 20, 40, 50, 30, 10, 10]",
		},
		{
			"tuple",
			SEEDED,
			"(10, 20, 30, 40, 50)",
			"[40, 20, 40, 30, 50, 20, 20, 40, 50, 30, 10, 10]",
		},
		{
			"range with a step",
			SEEDED,
			"range(5, 55, 10)",
			"[35, 15, 35, 25, 45, 15, 15, 35, 45, 25, 5, 5]",
		},
		{
			"range of a thousand",
			SEEDED,
			"range(1000)",
			"[619, 376, 638, 512, 957, 266, 248, 757, 819, 497, 64, 51]",
		},
		{
			"descending range",
			SEEDED,
			"range(10, 0, -2)",
			"[4, 8, 4, 6, 2, 8, 8, 4, 2, 6, 10, 10]",
		},
		{
			"dict gives its keys",
			SEEDED,
			"{'a': 1, 'b': 2, 'c': 3, 'd': 4}",
			`["c", "d", "c", "c", "d", "b", "c", "d", "d", "b", "a", "b"]`,
		},
		{
			"set",
			SEEDED,
			"set([10, 20, 30])",
			"[20, 20, 20, 20, 30, 10, 10, 30, 30, 20, 10, 10]",
		},
		{
			"elems of a string",
			SEEDED,
			"'abcde'.elems()",
			`["d", "b", "d", "c", "e", "b", "b", "d", "e", "c", "a", "a"]`,
		},
		{
			"codepoints of a string",
			SEEDED,
			"'abcde'.codepoints()",
			`["d", "b", "d", "c", "e", "b", "b", "d", "e", "c", "a", "a"]`,
		},
		{
			"one element list",
			SEEDED,
			"[7]",
			"[7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7]",
		},
		{
			"one element range",
			SEEDED,
			"range(3, 4)",
			"[3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3]",
		},
		{
			"one element dict",
			SEEDED,
			"{'k': 1}",
			`["k", "k", "k", "k", "k", "k", "k", "k", "k", "k", "k", "k"]`,
		},
		{
			"the next draw after a list",
			AFTER,
			"[1, 2, 3]",
			"[3, 11]",
		},
		{
			"the next draw after a range",
			AFTER,
			"range(3)",
			"[2, 11]",
		},
		{
			"the next draw after a dict",
			AFTER,
			"{'a': 1, 'b': 2, 'c': 3}",
			`["c", 11]`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					body = fmt.Sprintf(
						item.script,
						item.sequence,
					)
					got, err = _Expression(
						t,
						body,
					)
				)
				if err != nil {
					t.Fatal(err)
				}

				if got.String() != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						body,
						got.String(),
						item.want,
					)
				}
			},
		)
	}
}

// TestChoice_ErrorTextStaysTheSame pins the error text of a choice from
// nothing and from a value that is not iterable. The want values were
// captured from the code that copied the iterable first.
//
// Revisions:
//   - 2026-11-02 11:30: initial creation
//   - 2026-11-04 06:39: drops the position a run-time error now begins with (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestChoice_ErrorTextStaysTheSame(t *testing.T) {
	cases := []struct {
		name     string
		sequence string
		want     string
	}{
		{
			"empty list",
			"[]",
			"random.choice: nothing to choose from",
		},
		{
			"empty tuple",
			"()",
			"random.choice: nothing to choose from",
		},
		{
			"empty range",
			"range(0)",
			"random.choice: nothing to choose from",
		},
		{
			"empty dict",
			"{}",
			"random.choice: nothing to choose from",
		},
		{
			"empty set",
			"set()",
			"random.choice: nothing to choose from",
		},
		{
			"empty elems",
			"''.elems()",
			"random.choice: nothing to choose from",
		},
		{
			"a string",
			"'abc'",
			"random.choice: for parameter 1: got string, want iterable",
		},
		{
			"an int",
			"3",
			"random.choice: for parameter 1: got int, want iterable",
		},
		{
			"bytes",
			"b'abc'",
			"random.choice: for parameter 1: got bytes, want iterable",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					body = fmt.Sprintf(
						SINGLE,
						item.sequence,
					)
					_, err = _Expression(
						t,
						body,
					)
				)
				if err == nil {
					t.Fatalf(
						"%s gave no error",
						body,
					)
				}

				// The position of the call is not what this pins.
				if PLACE.ReplaceAllString(
					err.Error(),
					"",
				) != item.want {
					t.Fatalf(
						"%s gave %q, want %q",
						body,
						err.Error(),
						item.want,
					)
				}
			},
		)
	}
}

// BenchmarkChoice measures a run that draws one choice per loop turn from an
// iterable of a size, and reports allocations.
//
// Revisions:
//   - 2026-11-02 11:30: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkChoice(b *testing.B) {
	for _, source := range []string{
		"list(range(1000))",
		"tuple(range(1000))",
		"range(1000)",
		"{i: i for i in range(1000)}",
	} {
		b.Run(
			source,
			func(b *testing.B) {
				var (
					text = strings.Join(
						[]string{
							"def main():",
							"    items = " + source,
							"    for i in range(" +
								strconv.Itoa(CALLS) + "):",
							"        random.choice(items)",
							"",
						},
						"\n",
					)
					art, err = runtime.Compile(&common.Source{
						Entry: "choice_bench.star",
						Text:  []byte(text),
					})
				)
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						art,
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = run.Wait()
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
