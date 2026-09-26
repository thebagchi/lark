package random_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/core"
	"github.com/thebagchi/lark/v1/runtime/plugin/random"
)

// FIXTURES is where the scripts these tests run live.
const FIXTURES = "testdata"

// _Ran compiles a fixture and returns what it produced.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Ran(t *testing.T, name string) starlark.Value {
	t.Helper()

	path := filepath.Join(FIXTURES, name)

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	art, err := runtime.NewCompiler().Compile(path, src)
	if err != nil {
		t.Fatalf("compile %s: %v", name, err)
	}

	got, err := art.Run(t.Context())
	if err != nil {
		t.Fatalf("run %s: %v", name, err)
	}

	return got
}

// _Expression evaluates one expression inside a run, so that the source is
// found on a thread the scheduler set up.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Expression(t *testing.T, body string) (starlark.Value, error) {
	t.Helper()

	src := "def main():\n    " + body + "\n"

	art, err := runtime.NewCompiler().Compile("random_test.star", []byte(src))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	return art.Run(t.Context())
}

// TestSeed_RepeatsOnOneThread is what seeding is for, and the limit of what it
// promises: one thread, one seed, the same sequence.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestSeed_RepeatsOnOneThread(t *testing.T) {
	first := _Ran(t, "seeded.star")
	second := _Ran(t, "seeded.star")

	if first.String() != second.String() {
		t.Fatalf("one seed gave %s then %s", first.String(), second.String())
	}

	t.Logf("seeded: %s", first.String())
}

// TestSource_BelongsToOneRun checks that two runs of one artifact draw
// independently when nobody seeds them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestSource_BelongsToOneRun(t *testing.T) {
	src := []byte("def main():\n    return [random.int(1, 1000000000) for i in range(8)]\n")

	art, err := runtime.NewCompiler().Compile("unseeded.star", src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	first, err := art.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	second, err := art.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if first.String() == second.String() {
		t.Fatalf("two unseeded runs both gave %s", first.String())
	}
}

// TestSource_IsSafeFromEveryThreadAtOnce is the lock, checked by drawing from
// four threads at once under the race detector.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestSource_IsSafeFromEveryThreadAtOnce(t *testing.T) {
	got := _Ran(t, "threaded.star")

	held, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("returned a %T, want a list", got)
	}

	if held.Len() != 4 {
		t.Fatalf("drew from %d threads, want 4", held.Len())
	}
}

// TestRandom_StaysInsideWhatItWasAsked checks the ends of the range, which a
// generator that was one out would break.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestRandom_StaysInsideWhatItWasAsked(t *testing.T) {
	got, err := _Expression(t, "return [random.int(3, 5) for i in range(200)]")
	if err != nil {
		t.Fatal(err)
	}

	held, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("returned a %T, want a list", got)
	}

	seen := map[string]bool{}

	for index := range held.Len() {
		seen[held.Index(index).String()] = true
	}

	for _, want := range []string{"3", "4", "5"} {
		if !seen[want] {
			t.Fatalf("200 draws from 3 to 5 never gave %s: %v", want, seen)
		}
	}

	if len(seen) != 3 {
		t.Fatalf("drew %v, want only 3, 4 and 5", seen)
	}
}

// TestRandom_ShufflesWithoutTouchingWhatItWasGiven records that a shuffle is a
// new list, since module scope is frozen and reordering in place would fail
// exactly where a script most wants it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestRandom_ShufflesWithoutTouchingWhatItWasGiven(t *testing.T) {
	got, err := _Expression(t, `
    held = [1, 2, 3, 4, 5, 6, 7, 8]
    mixed = random.shuffle(held)
    return [held, sorted(mixed)]`)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != `[[1, 2, 3, 4, 5, 6, 7, 8], [1, 2, 3, 4, 5, 6, 7, 8]]` {
		t.Fatalf("got %s, want the original untouched and the shuffle a permutation", got.String())
	}
}

// TestRandom_RefusesWhatItCannotDo records the mistakes a caller can make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestRandom_RefusesWhatItCannotDo(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{"a backwards range", "return random.int(9, 1)", random.ErrRange},
		{"a choice from nothing", "return random.choice([])", random.ErrEmpty},
		{"negative bytes", "return random.bytes(-1)", random.ErrRange},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Expression(t, item.body)
			if !errors.Is(err, item.want) {
				t.Fatalf("got %v, want %v", err, item.want)
			}
		})
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
func TestInt_SpansTheWholeRange(t *testing.T) {
	cases := map[string]string{
		"to the top":        "return random.int(0, 9223372036854775807)",
		"the whole of it":   "return random.int(-9223372036854775808, 9223372036854775807)",
		"from the bottom":   "return random.int(-9223372036854775808, 0)",
		"one short of full": "return random.int(-9223372036854775807, 9223372036854775807)",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := _Expression(t, "random.seed(1)\n    "+body)
			if err != nil {
				t.Fatalf("%s gave %v", body, err)
			}

			if got == starlark.None {
				t.Fatalf("%s gave None", body)
			}
		})
	}
}

// TestInt_KeepsTheOrdinaryRanges checks the fix did not disturb the ranges a
// script actually writes, including the two edges.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
func TestInt_KeepsTheOrdinaryRanges(t *testing.T) {
	got, err := _Expression(t, "return [random.int(0, 0), random.int(-1, -1), random.int(4, 4)]")
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "[0, -1, 4]" {
		t.Fatalf("got %s, want each single-value range to give its one value", got.String())
	}
}
