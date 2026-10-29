package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
)

const (
	SETGET_SCRIPT  = "bench_setget.star"
	THREADS_SCRIPT = "bench_threads.star"
)

// BenchmarkContend_TwoThreadsOnOneName measures how long a state.set blocks
// while a state.update holds the same name. The update sleeps 50 ms. The set
// starts 10 ms into that sleep. The blocked time is the run duration minus the
// update sleep.
//
// Revisions:
//   - 2026-10-24 17:52: initial creation
func BenchmarkContend_TwoThreadsOnOneName(b *testing.B) {
	var (
		name = "bench_contend.star"
		src  []byte
		err  error
	)

	src, err = os.ReadFile(filepath.Join(
		FIXTURE_DIR,
		name,
	))
	if err != nil {
		b.Fatal(err)
	}

	built, err := runtime.Compile(&runtime.Source{
		Entry:  name,
		Text:   src,
		Loader: &_Disk{},
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for range b.N {
		started := time.Now()

		_, err = runtime.Start(
			b.Context(),
			built,
		).Wait()
		if err != nil {
			b.Fatal(err)
		}

		b.ReportMetric(
			float64(time.Since(started).Milliseconds()),
			"ms/op",
		)
	}
}

// BenchmarkState_SetGet measures one thread that does 1000 pairs of state.set
// and state.get on one name, so no other thread waits on the lock.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkState_SetGet(b *testing.B) {
	_Measure(
		b,
		SETGET_SCRIPT,
	)
}

// BenchmarkState_Threads measures 16 spawned threads that each do 100
// state.set on one name, then the join in main. The threads contend for the
// lock of that name.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkState_Threads(b *testing.B) {
	_Measure(
		b,
		THREADS_SCRIPT,
	)
}

// _Measure compiles the named fixture before the timer, then runs it once per
// iteration and ends the benchmark at the first error.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func _Measure(
	b *testing.B,
	name string,
) {
	b.Helper()

	src, err := os.ReadFile(filepath.Join(
		FIXTURE_DIR,
		name,
	))
	if err != nil {
		b.Fatal(err)
	}

	built, err := runtime.Compile(&runtime.Source{
		Entry:  name,
		Text:   src,
		Loader: &_Disk{},
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
