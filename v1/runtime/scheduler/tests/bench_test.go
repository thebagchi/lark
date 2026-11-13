package tests

import (
	"strconv"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// BENCH_NAME is the name of the script and of the run each benchmark
	// starts.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-12 18:58: names the script bench_fork.star, formerly
	//     bench_beside.star (C129, Q396)
	BENCH_NAME = "bench_fork.star"

	// WORK is the script whose function every spawn runs. It returns at once,
	// so the cost measured is the thread, not its work.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WORK = `
def work():
    return 1
`

	// WORKER_NAME is the name of the function in WORK.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WORKER_NAME = "work"
)

var (
	// SPAWNS are the thread counts of BenchmarkFork_NoReporter, the counts
	// of BenchmarkSpawn_Join in the runtime package.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-12 18:58: uses BenchmarkFork_NoReporter, formerly BenchmarkBeside_NoReporter
	//     (C129, Q396)
	SPAWNS = []int{
		1,
		16,
		256,
	}
)

// BenchmarkFork_NoReporter measures a run that starts a count of threads with
// Fork and waits for each, with no reporter in its settings. The recorder of
// the observe package is not there, so the cost is the scheduler's own: the
// Fork and Wait calls and the thread each starts.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
//   - 2026-11-12 18:58: renamed from BenchmarkBeside_NoReporter (C129, Q396)
func BenchmarkFork_NoReporter(b *testing.B) {
	work := _Work(b)

	for _, count := range SPAWNS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					_, err := scheduler.Eval(
						b.Context(),
						BENCH_NAME,
						_Body(
							work,
							count,
						),
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _Body is the body of a run that starts count threads of work and waits for
// them.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Body(
	work starlark.Callable,
	count int,
) func(thread *starlark.Thread) (starlark.Value, error) {
	return func(thread *starlark.Thread) (starlark.Value, error) {
		return nil, _SpawnWait(
			thread,
			work,
			count,
		)
	}
}

// _SpawnWait starts count threads that run work and waits for each, in turn.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func _SpawnWait(
	thread *starlark.Thread,
	work starlark.Callable,
	count int,
) error {
	handles := make(
		[]common.Handle,
		0,
		count,
	)

	for range count {
		handle, err := scheduler.Fork(
			thread,
			work,
		)
		if err != nil {
			return err
		}

		handles = append(
			handles,
			handle,
		)
	}

	for _, handle := range handles {
		_, err := scheduler.Wait(
			thread,
			handle,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Work is the function of WORK, compiled and frozen before the timer.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Work(b *testing.B) starlark.Callable {
	b.Helper()

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: BENCH_NAME,
		},
		BENCH_NAME,
		[]byte(WORK),
		starlark.StringDict{},
	)
	if err != nil {
		b.Fatal(err)
	}

	globals.Freeze()

	work, ok := globals[WORKER_NAME].(starlark.Callable)
	if !ok {
		b.Fatalf(
			"%s is not callable",
			WORKER_NAME,
		)
	}

	return work
}

// BenchmarkLock_TakeAndRelease is one Lock and its release on a thread that
// holds nothing, the cost of each update of a state loop.
//
// Revisions:
//   - 2026-11-05 22:05: initial creation (C58 fix round 1)
//   - 2026-11-07 20:07: uses common.Hold, common.Evaluation, common.Handle and scheduler.Release
//     (C91w5, Q127, Q229)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkLock_TakeAndRelease(b *testing.B) {
	var (
		thread = &starlark.Thread{}
		finish = scheduler.Begin(
			b.Context(),
			thread,
			common.ENTRY,
			nil,
		)
	)

	defer func() {
		_ = finish(nil)
	}()

	b.ReportAllocs()

	for b.Loop() {
		release, err := scheduler.Lock(
			thread,
			"counter",
		)
		if err != nil {
			b.Fatal(err)
		}

		scheduler.Release(release)
	}
}
