package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/random"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// SEEDED_SCRIPT is the text of the module seeded.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SEEDED_SCRIPT = `
# Draws a fixed number of values from a seeded source, on one thread, which is
# where a seed repeats exactly.

def main():
    random.seed(42)

    return [random.int(1, 1000000) for i in range(10)]
`

	// THREADED is the text of the module threaded.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	THREADED = `
# Draws from several threads at once, which is what the lock is for.

def draw():
    return [random.int(1, 1000000) for i in range(50)]

def main():
    return join(spawn(draw), spawn(draw), spawn(draw), spawn(draw))
`
)

var (
	// MODULES maps each module name to its text. _Ran reads it where the tests
	// once read testdata.
	//
	// Revisions:
	//   - 2026-11-09 11:16: initial creation (C114, Q281)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MODULES = map[string]string{
		"seeded.star":   _Script(SEEDED_SCRIPT),
		"threaded.star": _Script(THREADED),
	}
)

// _Ran compiles a fixture and returns what it produced.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads MODULES, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ran(
	t *testing.T,
	name string,
) starlark.Value {
	t.Helper()

	src, ok := MODULES[name]
	if !ok {
		t.Fatalf(
			"no fixture %s",
			name,
		)
	}

	art, err := runtime.Compile(&common.Source{
		Entry: name,
		Text:  []byte(src),
	})
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		art,
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatalf(
			"run %s: %v",
			name,
			err,
		)
	}

	return got
}

// _Expression evaluates one expression inside a run, so that the source is
// found on a thread the scheduler set up.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Expression(
	t *testing.T,
	body string,
) (starlark.Value, error) {
	t.Helper()

	var (
		src      = "def main():\n    " + body + "\n"
		art, err = runtime.Compile(&common.Source{
			Entry: "random_test.star",
			Text:  []byte(src),
		})
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		art,
	)
	if err != nil {
		t.Fatal(err)
	}

	return run.Wait()
}

// TestSeed_RepeatsOnOneThread is what seeding is for, and the limit of what it
// promises: one thread, one seed, the same sequence.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSeed_RepeatsOnOneThread(t *testing.T) {
	var (
		first = _Ran(
			t,
			"seeded.star",
		)
		second = _Ran(
			t,
			"seeded.star",
		)
	)

	if first.String() != second.String() {
		t.Fatalf(
			"one seed gave %s then %s",
			first.String(),
			second.String(),
		)
	}

	t.Logf(
		"seeded: %s",
		first.String(),
	)
}

// TestSource_BelongsToOneRun checks that two runs of one artifact draw
// independently when nobody seeds them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSource_BelongsToOneRun(t *testing.T) {
	var (
		src = []byte("def" +
			" main():\n    return [random.int(1, 1000000000) for i in " +
			"range(8)]\n")
		art, err = runtime.Compile(&common.Source{
			Entry: "unseeded.star",
			Text:  src,
		})
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		art,
	)
	if err != nil {
		t.Fatal(err)
	}

	first, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	run, err = runtime.Start(
		t.Context(),
		art,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if first.String() == second.String() {
		t.Fatalf(
			"two unseeded runs both gave %s",
			first.String(),
		)
	}
}

// TestSource_IsSafeFromEveryThreadAtOnce is the lock, checked by drawing from
// four threads at once under the race detector.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestSource_IsSafeFromEveryThreadAtOnce(t *testing.T) {
	var (
		got = _Ran(
			t,
			"threaded.star",
		)
		held, ok = got.(*starlark.List)
	)
	if !ok {
		t.Fatalf(
			"returned a %T, want a list",
			got,
		)
	}

	if held.Len() != 4 {
		t.Fatalf(
			"drew from %d threads, want 4",
			held.Len(),
		)
	}
}

// TestRandom_StaysInsideWhatItWasAsked checks the ends of the range, which a
// generator that was one out would break.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRandom_StaysInsideWhatItWasAsked(t *testing.T) {
	got, err := _Expression(
		t,
		"return [random.int(3, 5) for i in range(200)]",
	)
	if err != nil {
		t.Fatal(err)
	}

	held, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf(
			"returned a %T, want a list",
			got,
		)
	}

	seen := map[string]bool{}

	for index := range held.Len() {
		seen[held.Index(index).String()] = true
	}

	for _, want := range []string{"3", "4", "5"} {
		if !seen[want] {
			t.Fatalf(
				"200 draws from 3 to 5 never gave %s: %v",
				want,
				seen,
			)
		}
	}

	if len(seen) != 3 {
		t.Fatalf(
			"drew %v, want only 3, 4 and 5",
			seen,
		)
	}
}

// TestRandom_ShufflesWithoutTouchingWhatItWasGiven records that a shuffle is a
// new list, since module scope is frozen and reordering in place would fail
// exactly where a script most wants it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRandom_ShufflesWithoutTouchingWhatItWasGiven(t *testing.T) {
	got, err := _Expression(
		t,
		`
    held = [1, 2, 3, 4, 5, 6, 7, 8]
    mixed = random.shuffle(held)
    return [held, sorted(mixed)]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != `[[1, 2, 3, 4, 5, 6, 7, 8], [1, 2, 3, 4, 5, 6, 7, 8]]` {
		t.Fatalf(
			"got %s, want the original untouched and the shuffle a permutation",
			got.String(),
		)
	}
}

// TestRandom_RefusesWhatItCannotDo records the mistakes a caller can make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRandom_RefusesWhatItCannotDo(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{
			"a backwards range",
			"return random.int(9, 1)",
			unpack.ErrRange,
		},
		{
			"a choice from nothing",
			"return random.choice([])",
			random.ErrEmpty,
		},
		{
			"negative bytes",
			"return random.bytes(-1)",
			unpack.ErrRange,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Expression(
					t,
					item.body,
				)
				if !errors.Is(
					err,
					item.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestInt_SpansTheWholeRange is the width that does not fit in the type it was
// being computed in.
//
// high - low + 1 is one more than an int64 holds when the range is the whole
// of int64, so it wrapped to the most negative value and the generator panicked
// on it. The panic guard turned that into a failed run rather than a dead
// host, which is the runtime working - but a script author was told "invalid
// argument to Int64N", which names nothing they wrote.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestInt_SpansTheWholeRange(t *testing.T) {
	cases := map[string]string{
		"to the top": "return random.int(0, 9223372036854775807)",
		"the whole of it": "return random.int(" +
			"-9223372036854775808, 9223372036854775807)",
		"from the bottom": "return random.int(-9223372036854775808, 0)",
		"one short of full": "return random.int(" +
			"-9223372036854775807, 9223372036854775807)",
	}

	for name, body := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got, err := _Expression(
					t,
					"random.seed(1)\n    "+body,
				)
				if err != nil {
					t.Fatalf(
						"%s gave %v",
						body,
						err,
					)
				}

				if got == starlark.None {
					t.Fatalf(
						"%s gave None",
						body,
					)
				}
			},
		)
	}
}

// TestInt_KeepsTheOrdinaryRanges checks the fix did not disturb the ranges a
// script actually writes, including the two edges.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestInt_KeepsTheOrdinaryRanges(t *testing.T) {
	got, err := _Expression(
		t,
		"return [random.int(0, 0), random.int(-1, -1), random.int(4, 4)]",
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "[0, -1, 4]" {
		t.Fatalf(
			"got %s, want each single-value range to give its one value",
			got.String(),
		)
	}
}

// _Script returns text without the newline that opens its raw string.
//
// Revisions:
//   - 2026-11-09 11:36: initial creation (C114, Q289)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Script(text string) string {
	return strings.TrimPrefix(
		text,
		"\n",
	)
}
