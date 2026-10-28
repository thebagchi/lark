package flow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
)

// BenchmarkRetry_EmptyBody measures the cost of a retry with an empty body
// and delay 0. Each attempt calls Beside, which charges THREAD_COST and
// starts a goroutine.
//
// Revisions:
//   - 2026-10-24 17:52: initial creation
func BenchmarkRetry_EmptyBody(b *testing.B) {
	var (
		name = "bench_retry.star"
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
		_, err = runtime.Start(
			b.Context(),
			built,
		).Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}
