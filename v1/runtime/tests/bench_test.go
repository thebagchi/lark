package tests

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// RETURN is the smallest run: main returns a constant and spawns nothing.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETURN = `
def main():
    return 1
`
	RETURN_ENTRY = "bench_return.star"

	// SPAWN spawns its count of threads that each return 1, then joins them
	// all in one call. The %d is the count.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	STATEMENTS = `
def leaf(i):
    return i

def main():
    for i in range(%[1]d):
        leaf(i)

    return %[1]d
`
	STATEMENTS_ENTRY = "bench_statements.star"

	// THREADED spawns THREADS threads that each call a small script function
	// as a statement a count of times, then joins them. The %d is the count
	// of calls of each thread.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	THREADED = `
def leaf(i):
    return i

def work():
    for i in range(%d):
        leaf(i)

    return 1

def main():
    handles = []

    for _ in range(8):
        handles.append(spawn(work))

    return len(join(*handles))
`
	THREADED_ENTRY = "bench_threaded.star"
	THREADED_CALLS = 500

	// SPINNING spawns its count of threads that each loop until the run is
	// cancelled, then joins them. The %d is the count.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPINNING = `
def spin():
    for _ in range(1000000000):
        pass

    return 1

def main():
    handles = []

    for _ in range(%d):
        handles.append(spawn(spin))

    return len(join(*handles))
`
	SPINNING_ENTRY = "bench_spinning.star"

	// LOOP_PASS and LOOP_BUILTIN run one thread through a loop of %d
	// iterations, and return. The first does nothing in the body; the second
	// calls the builtin len, which allocates. They measure what the thread's
	// cancel poll costs a run nobody cancels.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LOOP_PASS = `
def main():
    for _ in range(%d):
        pass

    return 1
`
	LOOP_PASS_ENTRY = "bench_loop_pass.star"
	LOOP_BUILTIN    = `
def main():
    for _ in range(%d):
        len("abc")

    return 1
`
	LOOP_BUILTIN_ENTRY = "bench_loop_builtin.star"
	LOOP_ITERATIONS    = 10000000

	// SPIN_BUILTIN is SPINNING with the builtin len in the loop, in place of
	// pass. The %d is the count.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPIN_BUILTIN = `
def spin():
    for _ in range(1000000000):
        len("abc")

    return 1

def main():
    handles = []

    for _ in range(%d):
        handles.append(spawn(spin))

    return len(join(*handles))
`
	SPIN_BUILTIN_ENTRY = "bench_spin_builtin.star"

	// LATENCY_THREADS is the count of spinning threads whose stop
	// BenchmarkCancel_Latency times.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LATENCY_THREADS = 256

	// LATENCY_SETTLE is how long the benchmark waits, after every thread
	// exists, for each to be running its loop.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LATENCY_SETTLE = 50 * time.Millisecond

	// SPINNING_LIMIT is how long the benchmark waits for the threads to exist.
	//
	// Revisions:
	//   - 2026-11-02 11:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPINNING_LIMIT = 30 * time.Second

	// FAILING spawns its count of threads that each work a little and then
	// fail an assertion, and joins them. The %d is the count. The run ends
	// with the first failure recorded, so the threads race to end it.
	//
	// Revisions:
	//   - 2026-11-02 11:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FAILING = `
def fail_late():
    for _ in range(200):
        pass

    assert(False, "failed")

def main():
    handles = []

    for _ in range(%d):
        handles.append(spawn(fail_late))

    return join(*handles)
`
	FAILING_ENTRY = "bench_failing.star"

	// CANCEL_AFTER is how long the run goes before the benchmark cancels it.
	// It is long enough for main to spawn every thread, and far shorter than
	// the loops would take.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	CANCEL_AFTER = 5 * time.Millisecond
)

var (
	// SPAWNS are the thread counts of BenchmarkSpawn_Join and
	// BenchmarkEval_SpawnJoin.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	//   - 2026-11-10 17:15: names BenchmarkEval_SpawnJoin, formerly
	//     BenchmarkEvaluate_SpawnJoin (C121, Q338)
	SPAWNS = []int{
		1,
		16,
		256,
	}

	// FANOUTS are the counts of running threads that BenchmarkCancel_Fanout
	// cancels.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	FANOUTS = []int{
		16,
		256,
	}

	// CALLS are the call counts of BenchmarkCall_Statements and
	// BenchmarkCall_Watched.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkRun_Return(b *testing.B) {
	built, err := runtime.Compile(&common.Source{
		Entry: RETURN_ENTRY,
		Text:  []byte(RETURN),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		run, err := runtime.Start(
			b.Context(),
			built,
		)
		if err != nil {
			b.Fatal(err)
		}

		_, err = run.Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEval_Return measures the fixed cost of one run evaluated on the
// caller's goroutine, with the script of BenchmarkRun_Return, so the two differ
// by the goroutine Start starts and the waiter it wakes.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: renamed from BenchmarkEvaluate_Return, with runtime.Evaluate now
//     runtime.Eval (C121, Q338)
func BenchmarkEval_Return(b *testing.B) {
	built, err := runtime.Compile(&common.Source{
		Entry: RETURN_ENTRY,
		Text:  []byte(RETURN),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err = runtime.Eval(
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
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkSpawn_Join(b *testing.B) {
	for _, count := range SPAWNS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
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

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						built,
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

// BenchmarkEval_SpawnJoin is BenchmarkSpawn_Join evaluated on the caller's
// goroutine, so the two differ by the run's own goroutine alone.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: renamed from BenchmarkEvaluate_SpawnJoin, with runtime.Evaluate now
//     runtime.Eval (C121, Q338)
func BenchmarkEval_SpawnJoin(b *testing.B) {
	for _, count := range SPAWNS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
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

				for b.Loop() {
					_, err = runtime.Eval(
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
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCall_Statements(b *testing.B) {
	for _, count := range CALLS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
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

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						built,
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

// BenchmarkCall_Watched is BenchmarkCall_Statements with a watcher that does
// nothing, so the two differ by what the recorder pays for a watched run: the
// path of each step. The allocations per call are the slope across the counts.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkCalls_Watched of
//     poc/research/artifact_observe_bench
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCall_Watched(b *testing.B) {
	for _, count := range CALLS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
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

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						built,
						runtime.WithChanges(_Ignore),
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

// _Ignore is a watcher that does nothing with what it is told.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ignore(change *workflowpb.Change) {
	_ = change
}

// BenchmarkCalls_Threads measures a run whose main spawns 8 threads that each
// call a script function as a statement THREADED_CALLS times, so the threads
// contend for the recorder's lock. Run it with -mutexprofile to see the share
// of the delay at the recorder.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkCalls_8Threads of
//     poc/research/artifact_observe_bench
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCalls_Threads(b *testing.B) {
	built, err := runtime.Compile(&common.Source{
		Entry: THREADED_ENTRY,
		Text: fmt.Appendf(
			nil,
			THREADED,
			THREADED_CALLS,
		),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		run, err := runtime.Start(
			b.Context(),
			built,
		)
		if err != nil {
			b.Fatal(err)
		}

		_, err = run.Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCancel_Fanout measures a run whose main spawns a count of threads
// that loop, and the cancel of the run CANCEL_AFTER later. The run must end
// with ErrCancelled, after every thread has stopped. The time includes
// CANCEL_AFTER, so compare the allocations, not the time.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCancel_Fanout(b *testing.B) {
	for _, count := range FANOUTS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
					Entry: SPINNING_ENTRY,
					Text: fmt.Appendf(
						nil,
						SPINNING,
						count,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()

				for b.Loop() {
					var (
						ctx, cancel = context.WithCancel(b.Context())
						timer       = time.AfterFunc(
							CANCEL_AFTER,
							cancel,
						)
					)

					run, err := runtime.Start(
						ctx,
						built,
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = run.Wait()

					timer.Stop()
					cancel()

					if !errors.Is(
						err,
						runtime.ErrCancelled,
					) {
						b.Fatalf(
							"want ErrCancelled, got %v",
							err,
						)
					}
				}
			},
		)
	}
}

// BenchmarkLoop_Pass measures a run of one thread that loops LOOP_ITERATIONS
// times and does nothing in the loop, so the cost of the thread's cancel poll
// is a share of the whole. Compare the allocations, not the time.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkLoop_Pass(b *testing.B) {
	_Loop(
		b,
		LOOP_PASS,
		LOOP_PASS_ENTRY,
	)
}

// BenchmarkLoop_Builtin is BenchmarkLoop_Pass with a call of the builtin len
// in the loop, which allocates, so a poll after every builtin shows here.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkLoop_Builtin(b *testing.B) {
	_Loop(
		b,
		LOOP_BUILTIN,
		LOOP_BUILTIN_ENTRY,
	)
}

// _Loop measures runs of text, a script with LOOP_ITERATIONS in it, started
// and waited for.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Loop(
	b *testing.B,
	text string,
	entry string,
) {
	b.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: entry,
		Text: fmt.Appendf(
			nil,
			text,
			LOOP_ITERATIONS,
		),
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		run, err := runtime.Start(
			b.Context(),
			built,
		)
		if err != nil {
			b.Fatal(err)
		}

		_, err = run.Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCancel_Latency measures the time from the cancel of a run to the
// stop of the last of LATENCY_THREADS threads that spin in a loop, for a loop
// of pass and a loop of the builtin len.
//
// The metric to read is ns/cancel, the time from the cancel to the return of
// Wait. ns/op also holds the start of the run and LATENCY_SETTLE, because
// b.Loop ends on the timed total, and a timer stopped during the start would
// make it run for minutes.
//
// The threads are counted as goroutines, so run it with nothing else starting
// goroutines in the process.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCancel_Latency(b *testing.B) {
	cases := []struct {
		name  string
		entry string
		text  string
	}{
		{
			"pass",
			SPINNING_ENTRY,
			SPINNING,
		},
		{
			"builtin",
			SPIN_BUILTIN_ENTRY,
			SPIN_BUILTIN,
		},
	}

	for _, item := range cases {
		b.Run(
			item.name,
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
					Entry: item.entry,
					Text: fmt.Appendf(
						nil,
						item.text,
						LATENCY_THREADS,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				var stopped time.Duration

				for b.Loop() {
					var (
						running, cancel = _Spinning(
							b,
							built,
						)
						begun = time.Now()
					)

					cancel()

					_, err = running.Wait()
					stopped += time.Since(begun)

					if !errors.Is(
						err,
						runtime.ErrCancelled,
					) {
						b.Fatalf(
							"want ErrCancelled, got %v",
							err,
						)
					}
				}

				b.ReportMetric(
					float64(stopped.Nanoseconds())/float64(b.N),
					"ns/cancel",
				)
			},
		)
	}
}

// _Spinning starts built and returns it, with the function that cancels it,
// once LATENCY_THREADS threads beside the run's own are running, and have had
// LATENCY_SETTLE to start their loops.
//
// Revisions:
//   - 2026-11-02 11:56: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Spinning(
	b *testing.B,
	built common.Artifact,
) (common.Execution, context.CancelFunc) {
	b.Helper()

	var (
		ctx, cancel = context.WithCancel(b.Context())
		before      = goruntime.NumGoroutine()
		deadline    = time.Now().Add(SPINNING_LIMIT)
	)

	running, err := runtime.Start(
		ctx,
		built,
	)
	if err != nil {
		b.Fatal(err)
	}

	for goruntime.NumGoroutine() < before+LATENCY_THREADS {
		if time.Now().After(deadline) {
			b.Fatal("the threads did not start")
		}

		time.Sleep(time.Millisecond)
	}

	time.Sleep(LATENCY_SETTLE)

	return running, cancel
}

// BenchmarkFail_Threads measures a run whose main spawns a count of threads
// that fail at nearly the same time. Every one of them ends the run, and the
// run must return an error. Run it with -mutexprofile to see the delay at the
// run's lock.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkFail_Threads(b *testing.B) {
	for _, count := range FANOUTS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
					Entry: FAILING_ENTRY,
					Text: fmt.Appendf(
						nil,
						FAILING,
						count,
					),
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						built,
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = run.Wait()
					if err == nil {
						b.Fatal("a failing run returned no error")
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
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCompile_Functions(b *testing.B) {
	src := _Functions(FUNCTIONS)

	b.ReportAllocs()

	for b.Loop() {
		_, err := runtime.Compile(&common.Source{
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
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
