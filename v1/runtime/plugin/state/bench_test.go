package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
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
