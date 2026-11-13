package tests

import (
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// BENCH_CONTEND is the text of the module bench_contend.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH_CONTEND = `
def holder():
    state.update("k", lambda v: sleep(50))

def setter():
    sleep(10)
    state.set("k", 1)

def main():
    spawn(holder)
    spawn(setter)
    sleep(100)
    return state.get("k")
`

	// GETMANY_SCRIPT is the text of the module bench_getmany.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	GETMANY_SCRIPT = `
# Sixteen threads each read sixteen names, 100 reads in all. Nothing writes
# after main stores the names, so every state call is a read.
NAMES = ["k%d" % i for i in range(16)]

def reader():
    for i in range(100):
        state.get(NAMES[i % 16])

def main():
    for name in NAMES:
        state.set(name, 1)

    handles = []

    for _ in range(16):
        handles.append(spawn(reader))

    join(*handles)

    return state.get("k0")
`

	// SETGET_SCRIPT is the text of the module bench_setget.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SETGET_SCRIPT = `
# One thread writes a name and reads it back, as pairs in a loop.
def main():
    for i in range(1000):
        state.set("k", i)
        seen = state.get("k")

    return seen
`

	// THREADS_SCRIPT is the text of the module bench_threads.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	THREADS_SCRIPT = `
# Sixteen threads write the same name, each in a loop.
def writer():
    for i in range(100):
        state.set("k", i)

def main():
    handles = []

    for _ in range(16):
        handles.append(spawn(writer))

    join(*handles)

    return state.get("k")
`
)

// BenchmarkContend_TwoThreadsOnOneName measures how long a state.set blocks
// while a state.update holds the same name. The update sleeps 50 ms. The set
// starts 10 ms into that sleep. The blocked time is the run duration minus the
// update sleep.
//
// Revisions:
//   - 2026-10-24 17:52: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-05 17:15: reports ms/op once, as the mean of all runs, not the last run (B044, C96)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads the script const or MODULES, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkContend_TwoThreadsOnOneName(b *testing.B) {
	var name = "bench_contend.star"

	built, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   []byte(_Script(BENCH_CONTEND)),
		Loader: &_Memory{},
	})
	if err != nil {
		b.Fatal(err)
	}

	var total time.Duration

	for b.Loop() {
		var (
			started  = time.Now()
			run, err = runtime.Start(
				b.Context(),
				built,
			)
		)
		if err != nil {
			b.Fatal(err)
		}

		_, err = run.Wait()
		if err != nil {
			b.Fatal(err)
		}

		total += time.Since(started)
	}

	// Once, after the loop: ReportMetric sets the value, it does not add to it.
	b.ReportMetric(
		float64(total.Microseconds())/1000/float64(b.N),
		"ms/op",
	)
}

// BenchmarkState_SetGet measures one thread that does 1000 pairs of state.set
// and state.get on one name, so no other thread waits on the lock.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkState_SetGet(b *testing.B) {
	_Measure(
		b,
		"bench_setget.star",
	)
}

// BenchmarkState_Threads measures 16 spawned threads that each do 100
// state.set on one name, then the join in main. The threads contend for the
// lock of that name.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkState_Threads(b *testing.B) {
	_Measure(
		b,
		"bench_threads.star",
	)
}

// BenchmarkState_GetManyThreads measures 16 spawned threads that each do 100
// state.get over 16 names, after main stores the names. No thread writes, so
// the contention is on the run's lookup of the store, not on a name's lock.
// Run it with -mutexprofile to see where the threads wait.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-09 17:03: names the script by its module name (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkState_GetManyThreads(b *testing.B) {
	_Measure(
		b,
		"bench_getmany.star",
	)
}

// _Measure compiles the named fixture before the timer, then runs it once per
// iteration and ends the benchmark at the first error.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads the script const or MODULES, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Measure(
	b *testing.B,
	name string,
) {
	b.Helper()

	src, ok := MODULES[name]
	if !ok {
		b.Fatalf(
			"no fixture %s",
			name,
		)
	}

	built, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   []byte(src),
		Loader: &_Memory{},
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
