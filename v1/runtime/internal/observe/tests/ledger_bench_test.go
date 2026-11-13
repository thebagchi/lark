package tests

import (
	"fmt"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

const (
	// ONE_THREAD is one thread making its count of statement calls.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ONE_THREAD = `
def leaf(i):
    return i

def main():
    for i in range(%d):
        leaf(i)
    return 1
`

	// EIGHT_THREADS is eight spawned threads making their count of statement
	// calls each.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	EIGHT_THREADS = `
def leaf(i):
    return i

def work():
    for i in range(%d):
        leaf(i)
    return 1

def main():
    hs = []
    for _ in range(8):
        hs.append(spawn(work))
    return len(join(*hs))
`

	// SPAWNS spawns its count of threads that each return, then joins them.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPAWNS = `
def work():
    return 1

def main():
    hs = []
    for _ in range(%d):
        hs.append(spawn(work))
    return len(join(*hs))
`
)

// _Measured runs the script text, filled with n, b.N times with opts and
// reports allocations. The default run keeps the ledger; WithTracking keeps
// the recorder.
//
// Revisions:
//   - 2026-11-02 18:02: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func _Measured(
	b *testing.B,
	text string,
	n int,
	opts ...common.RunOption,
) {
	b.Helper()

	art, err := runtime.Compile(&common.Source{
		Entry: "bench.star",
		Text: []byte(fmt.Sprintf(
			text,
			n,
		)),
	})
	if err != nil {
		b.Fatal(err)
	}

	all := append(
		[]common.RunOption{_Quiet()},
		opts...,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		run, err := observe.Start(
			b.Context(),
			art,
			all...,
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

// BenchmarkEndGraph_OneThread_Default is one thread, 4000 calls, no live graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEndGraph_OneThread_Default(b *testing.B) {
	_Measured(
		b,
		ONE_THREAD,
		4000,
	)
}

// BenchmarkEndGraph_OneThread_Live is the same run with the live graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func BenchmarkEndGraph_OneThread_Live(b *testing.B) {
	_Measured(
		b,
		ONE_THREAD,
		4000,
		artifact.WithTracking(),
	)
}

// BenchmarkEndGraph_EightThreads_Default is eight threads, 500 calls each, no
// live graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEndGraph_EightThreads_Default(b *testing.B) {
	_Measured(
		b,
		EIGHT_THREADS,
		500,
	)
}

// BenchmarkEndGraph_EightThreads_Live is the same run with the live graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func BenchmarkEndGraph_EightThreads_Live(b *testing.B) {
	_Measured(
		b,
		EIGHT_THREADS,
		500,
		artifact.WithTracking(),
	)
}

// BenchmarkEndGraph_Spawn256_Default is 256 threads that each return, no live
// graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEndGraph_Spawn256_Default(b *testing.B) {
	_Measured(
		b,
		SPAWNS,
		256,
	)
}

// BenchmarkEndGraph_Spawn256_Live is the same run with the live graph.
//
// Revisions:
//   - 2026-11-02 18:11: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func BenchmarkEndGraph_Spawn256_Live(b *testing.B) {
	_Measured(
		b,
		SPAWNS,
		256,
		artifact.WithTracking(),
	)
}
