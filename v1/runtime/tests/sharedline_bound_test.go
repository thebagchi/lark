package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _OneLine is a script whose first line holds count assignments, each of a name
// nothing reads, and whose main reads none.
//
// Revisions:
//   - 2026-11-12 23:48: initial creation (C130, B289, Q402)
func _OneLine(count int) string {
	var b strings.Builder

	for idx := range count {
		fmt.Fprintf(
			&b,
			"x%d = %d;",
			idx,
			idx,
		)
	}

	b.WriteString("\ndef main():\n    pass\n")

	return b.String()
}

// TestCompile_ManyStatementsOnOneLineIsBounded proves the compile of one line of
// 100,000 statements, all shaken, ends within 20 seconds: the time grows with the
// statements, where each place in the line was found by a walk from its start,
// and each statement asked again whether the text held a NUL, so it grew with
// their square (B289, Q402). A timing test is flaky only when its limit is near
// the time of the code. Measured with -race: 2.9 seconds on an idle machine
// (7 times under the limit), and 3.5 to 4.8 seconds with 16 busy loops on 8 cores
// (4 times under it); the walk from the line start takes about a minute at this
// size. The compile runs in a goroutine, so a failure is reported at the limit
// and not when the square ends. The second cost, the NUL check, is 4 seconds at
// this size, so this test does not see it return; the benchmark below does.
//
// The check: go test -race -count=1 -run TestCompile_ManyStatementsOnOneLineIsBounded
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 23:48: initial creation (C130, B289, Q402)
func TestCompile_ManyStatementsOnOneLineIsBounded(t *testing.T) {
	const (
		COUNT = 100000
		LIMIT = 20 * time.Second
	)

	var (
		source = &common.Source{
			Entry: "one_line.star",
			Text:  []byte(_OneLine(COUNT)),
		}
		done = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := runtime.Compile(source)

		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}

	case <-time.After(LIMIT):
		t.Fatalf(
			"the compile of %d statements on one line took over %v",
			COUNT,
			LIMIT,
		)
	}
}

// BenchmarkCompile_OneSharedLine is the compile of one line of 2,000, 20,000 and
// 200,000 statements that nothing reads (B289, Q402). The check:
// go test -run XXX -bench BenchmarkCompile_OneSharedLine -count=6
// ./v1/runtime/tests, compared with benchstat.
//
// Revisions:
//   - 2026-11-12 23:48: initial creation (C130, B289, Q402)
func BenchmarkCompile_OneSharedLine(b *testing.B) {
	for _, count := range []int{2000, 20000, 200000} {
		b.Run(
			fmt.Sprint(count),
			func(b *testing.B) {
				source := &common.Source{
					Entry: "one_line.star",
					Text:  []byte(_OneLine(count)),
				}

				b.ResetTimer()

				for range b.N {
					_, err := runtime.Compile(source)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
