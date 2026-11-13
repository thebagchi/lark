package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// BENCH_RETRY is the text of the module bench_retry.star.
	// The tests reach it through the in-memory loader.
	//
	// Revisions:
	//   - 2026-11-09 11:36: initial creation; leading newline (C114, Q281, Q287, Q289)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH_RETRY = `
def empty():
    return "done"

def main():
    return retry(1, empty)
`
)

// BenchmarkRetry_EmptyBody measures the cost of a retry with an empty body
// and delay 0. Each attempt calls Within, which runs it on the caller's thread
// and starts no goroutine.
//
// Revisions:
//   - 2026-10-24 17:52: initial creation
//   - 2026-11-01 13:00: the comment no longer names a thread charge
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-05 17:15: the comment names Within, which each attempt calls (B045, C96)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:16: reads the script const, not testdata (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkRetry_EmptyBody(b *testing.B) {
	var name = "bench_retry.star"

	built, err := runtime.Compile(&common.Source{
		Entry:  name,
		Text:   []byte(_Script(BENCH_RETRY)),
		Loader: &_Memory{},
	})
	if err != nil {
		b.Fatal(err)
	}

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
