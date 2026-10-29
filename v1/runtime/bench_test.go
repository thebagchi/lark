package runtime_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
)

const (
	// RETURN is the smallest run: main returns a constant and spawns nothing.
	RETURN = `
def main():
    return 1
`
	RETURN_ENTRY = "bench_return.star"

	// SPAWN spawns its count of threads that each return 1, then joins them
	// all in one call. The %d is the count.
	SPAWN = `
def work():
    return 1

def main():
    handles = []

    for _ in range(%d):
        handles.append(spawn(work))

    return len(join(*handles))
`
	SPAWN_ENTRY = "bench_spawn.star"

	// FUNCTION is one small function of the compile benchmark: a branch, a
	// list and a dict. The %d is the number in its name.
	FUNCTION = `
def f%d(x):
    if x > 0:
        return [x, x + 1]

    return {"x": x}
`
	FUNCTIONS       = 200
	FUNCTIONS_ENTRY = "bench_functions.star"
	FUNCTIONS_MAIN  = `
def main():
    return f0(1)
`

	// STATEMENTS calls a small script function as a statement its count of
	// times. A statement call goes through the dialect's call hook and the
	// recorder; a call in an expression does not. The %d is the count.
	STATEMENTS = `
def leaf(i):
    return i

def main():
    for i in range(%[1]d):
        leaf(i)

    return %[1]d
`
	STATEMENTS_ENTRY = "bench_statements.star"
)

var (
	// SPAWNS are the thread counts of BenchmarkSpawn_Join and
	// BenchmarkEvaluate_SpawnJoin.
	SPAWNS = []int{
		1,
		16,
		256,
	}

	// CALLS are the call counts of BenchmarkCall_Statements.
	CALLS = []int{
		1000,
		4000,
	}
)

// BenchmarkRun_Return measures the fixed cost of one run: start, the main
// thread, and the wait, with a script that does no work.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkRun_Return(b *testing.B) {
	built, err := runtime.Compile(&runtime.Source{
		Entry: RETURN_ENTRY,
		Text:  []byte(RETURN),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err = runtime.Start(
			b.Context(),
			built,
		).Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEvaluate_Return measures the fixed cost of one run evaluated on the
// caller's goroutine, with the script of BenchmarkRun_Return, so the two differ
// by the goroutine Start starts and the waiter it wakes.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func BenchmarkEvaluate_Return(b *testing.B) {
	built, err := runtime.Compile(&runtime.Source{
		Entry: RETURN_ENTRY,
		Text:  []byte(RETURN),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err = runtime.Evaluate(
			b.Context(),
			built,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSpawn_Join measures a run whose main spawns a count of threads and
// joins them, so the cost of one thread is the slope across the counts.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkSpawn_Join(b *testing.B) {
	for _, count := range SPAWNS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&runtime.Source{
					Entry: SPAWN_ENTRY,
					Text: fmt.Appendf(
						nil,
						SPAWN,
						count,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, err = runtime.Start(
						b.Context(),
						built,
					).Wait()
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkEvaluate_SpawnJoin is BenchmarkSpawn_Join evaluated on the caller's
// goroutine, so the two differ by the run's own goroutine alone.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func BenchmarkEvaluate_SpawnJoin(b *testing.B) {
	for _, count := range SPAWNS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&runtime.Source{
					Entry: SPAWN_ENTRY,
					Text: fmt.Appendf(
						nil,
						SPAWN,
						count,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, err = runtime.Evaluate(
						b.Context(),
						built,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkCall_Statements measures a run whose main calls a script function
// as a statement a count of times, so the cost of one recorded call is the
// slope across the counts.
//
// Revisions:
//   - 2026-10-29 00:21: initial creation
func BenchmarkCall_Statements(b *testing.B) {
	for _, count := range CALLS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&runtime.Source{
					Entry: STATEMENTS_ENTRY,
					Text: fmt.Appendf(
						nil,
						STATEMENTS,
						count,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, err = runtime.Start(
						b.Context(),
						built,
					).Wait()
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkCompile_Functions measures a compile alone, of a source with
// FUNCTIONS small functions, so the cost is the parse and the checks of each.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkCompile_Functions(b *testing.B) {
	src := _Functions(FUNCTIONS)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_, err := runtime.Compile(&runtime.Source{
			Entry: FUNCTIONS_ENTRY,
			Text:  src,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// _Functions is a source of count functions in the shape of FUNCTION, and a
// main that calls the first.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func _Functions(count int) []byte {
	var builder strings.Builder

	for idx := range count {
		fmt.Fprintf(
			&builder,
			FUNCTION,
			idx,
		)
	}

	builder.WriteString(FUNCTIONS_MAIN)

	return []byte(builder.String())
}
