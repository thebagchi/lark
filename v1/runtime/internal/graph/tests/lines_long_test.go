package tests

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestLines_AtOnALongLineIsTheWalkOfARuneSlice proves At returns, for every
// column of a line longer than LONG bytes, the byte offset a walk of its runes
// gives: ASCII, two-, three- and four-byte runes, the column after the last
// rune, and a column past it, on a line of a length that is a multiple of STRIDE
// runes and on others, between a short line and an empty one. The marks of a
// long line are a jump, and the walk after it must end at the same place (B289).
//
// The check: go test -race -count=1 -run TestLines_AtOnALongLineIsTheWalkOfARuneSlice
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 23:45: initial creation (C130, B289, Q402)
func TestLines_AtOnALongLineIsTheWalkOfARuneSlice(t *testing.T) {
	var kinds = []string{"a", "é", "日", "😀"}

	for _, runes := range []int{
		graph.LONG + 1,
		graph.STRIDE * 5,
		graph.STRIDE*5 + 1,
		graph.STRIDE*5 - 1,
		1000,
	} {
		for _, kind := range kinds {
			t.Run(
				fmt.Sprintf(
					"%s %d",
					kind,
					runes,
				),
				func(t *testing.T) {
					var (
						long = strings.Repeat(
							kind,
							runes,
						)
						src   = "short\n" + long + "\n\nend"
						lines = graph.NewLines(src)
						chars = []rune(long)
						walk  = len("short\n")
					)

					for col := 1; col <= len(chars)+2; col++ {
						want := graph.MISSING
						if col <= len(chars)+1 {
							want = walk
						}

						got := lines.At(
							2,
							int32(col),
						)
						if got != want {
							t.Fatalf(
								"%d %q, col %d: got %d, want %d",
								runes,
								kind,
								col,
								got,
								want,
							)
						}

						if col <= len(chars) {
							walk += len(string(chars[col-1]))
						}
					}

					if got := lines.At(
						1,
						3,
					); got != 2 {
						t.Fatalf(
							"a short line: got %d, want 2",
							got,
						)
					}

					if got := lines.At(
						4,
						3,
					); got != len(src)-1 {
						t.Fatalf(
							"the last line: got %d, want %d",
							got,
							len(src)-1,
						)
					}
				},
			)
		}
	}
}

// TestLines_AtIsSafeForGoroutines proves goroutines may ask one Lines for places
// on a long line at once: the marks are found once under a lock, and every
// answer is the same, as the Lines doc promises (run it with -race).
//
// The check: go test -race -count=1 -run TestLines_AtIsSafeForGoroutines
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 23:45: initial creation (C130, B289, Q402)
func TestLines_AtIsSafeForGoroutines(t *testing.T) {
	var (
		lines = graph.NewLines(strings.Repeat(
			"é",
			2000,
		))
		wait   sync.WaitGroup
		failed = make(
			chan int,
			8,
		)
	)

	for range 8 {
		wait.Add(1)

		go func() {
			defer wait.Done()

			for col := 1; col <= 2001; col++ {
				if got := lines.At(
					1,
					int32(col),
				); got != 2*(col-1) {
					failed <- col

					return
				}
			}
		}()
	}

	wait.Wait()
	close(failed)

	for col := range failed {
		t.Errorf(
			"column %d: a wrong offset",
			col,
		)
	}
}

// TestLines_AtOnALongLineIsBounded proves the cost of At on a long line does not
// grow with the line: 100,000 places on a line of a million runes are found in
// seconds at most, where a walk from the line start takes minutes (100,000 walks
// of half a million runes). A timing test is flaky only when the limit is near
// the time, and here the limit is 1,000 times the time of the code and a
// fraction of the time of the walk, so it fails for the square and not for load
// (B289, Q402).
//
// The check: go test -race -count=1 -run TestLines_AtOnALongLineIsBounded
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-12 23:45: initial creation (C130, B289, Q402)
func TestLines_AtOnALongLineIsBounded(t *testing.T) {
	const (
		RUNES = 1000000
		ASKS  = 100000
		LIMIT = 10 * time.Second
	)

	var (
		lines = graph.NewLines(strings.Repeat(
			"é",
			RUNES,
		))
		began = time.Now()
	)

	for ask := range ASKS {
		col := 1 + ask*(RUNES/ASKS)

		if got := lines.At(
			1,
			int32(col),
		); got != 2*(col-1) {
			t.Fatalf(
				"column %d: got %d",
				col,
				got,
			)
		}

		if took := time.Since(began); took > LIMIT {
			t.Fatalf(
				"%d places of %d on a line of %d runes took %v, want all under %v",
				ask,
				ASKS,
				RUNES,
				took,
				LIMIT,
			)
		}
	}
}

// BenchmarkLines_At_LongLine is the cost of one place on a line of 200,000
// statements' length, the shape of B289; run it with
// go test -run XXX -bench BenchmarkLines_At_LongLine -count=6
// ./v1/runtime/internal/graph/tests and compare with benchstat.
//
// Revisions:
//   - 2026-11-12 23:45: initial creation (C130, B289, Q402)
func BenchmarkLines_At_LongLine(b *testing.B) {
	var (
		runes = 2000000
		lines = graph.NewLines(strings.Repeat(
			"a",
			runes,
		))
	)

	b.ResetTimer()

	for ask := range b.N {
		lines.At(
			1,
			int32(1+(ask*977)%runes),
		)
	}
}
