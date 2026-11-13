package tests

import (
	"fmt"
	"math"
	goruntime "runtime"
	"runtime/metrics"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// RETENTION_HANDLES is how many finished handles the script keeps.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_HANDLES = 10000

	// RETENTION_CYCLES is how many collections _Live forces before it reads.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_CYCLES = 3

	// RETENTION_ROUNDS is how many times the test measures; it keeps the least.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_ROUNDS = 5

	// RETENTION_LIMIT is the most live heap one kept handle may hold, in bytes.
	// The recorded value before change C33 was 470 B per handle.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_LIMIT = 470

	// RETENTION_ENTRY is the name of the script of the retention test.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_ENTRY = "retention.star"

	// RETENTION_SCRIPT spawns its count of threads, joins them all and returns
	// the handles. Keep is 0 for the control run, which returns none. The first
	// %d is the count and the second is the keep flag.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_SCRIPT = `
def work():
    return 1

def main():
    handles = []

    for _ in range(%d):
        handles.append(spawn(work))

    join(*handles)

    if %d == 0:
        return []

    return handles
`

	// RETENTION_METRIC is the live heap the collector marked in its last cycle.
	//
	// Revisions:
	//   - 2026-11-02 16:14: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RETENTION_METRIC = "/gc/heap/live:bytes"
)

// _Live returns the live heap after a full collection, in bytes.
//
// The metric is the heap the last cycle marked, so floating garbage is not in
// it. HeapAlloc includes unreachable objects not yet swept, which made an
// earlier heap test flaky (poc/research/flaky_tests.md, cause 2).
//
// Revisions:
//   - 2026-11-02 12:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Live(t *testing.T) uint64 {
	t.Helper()

	// Three cycles: the first may be finishing when the call starts, and
	// the next frees what it left to the sweeper.
	for range RETENTION_CYCLES {
		goruntime.GC()
	}

	sample := []metrics.Sample{
		{
			Name: RETENTION_METRIC,
		},
	}

	metrics.Read(sample)

	if sample[0].Value.Kind() != metrics.KindUint64 {
		t.Fatalf(
			"metric %s has kind %v",
			RETENTION_METRIC,
			sample[0].Value.Kind(),
		)
	}

	return sample[0].Value.Uint64()
}

// _Retained runs the script and returns the live heap its result holds, in
// bytes, while the result is still reachable.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Retained(
	t *testing.T,
	keep int,
) int64 {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: RETENTION_ENTRY,
		Text: fmt.Appendf(
			nil,
			RETENTION_SCRIPT,
			RETENTION_HANDLES,
			keep,
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	var (
		before = _Live(t)
		run    common.Execution
	)

	run, err = runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err2 := run.Wait()
	if err2 != nil {
		t.Fatal(err2)
	}

	after := _Live(t)

	goruntime.KeepAlive(value)

	return int64(after) - int64(before)
}

// TestRetention_HeldHandles proves a finished handle a script keeps holds its
// result and not the thread or the run it came from.
//
// It runs the script twice: once keeping 10,000 handles and once keeping none.
// The difference is what the handles hold. The test keeps the least of
// RETENTION_ROUNDS rounds and passes when one handle holds at most
// RETENTION_LIMIT bytes, the value recorded before the change.
//
// Measured on this machine, with this command, on base e92f997 (after C29):
// about 310 B per handle before change C33 and about 346 B after it. Command:
// go test -count=1 -run Retention_HeldHandles -v ./v1/runtime
//
// Starting condition: a Go test binary with the race detector on or off. The
// check is the logged bytes per handle; run it with -v to see the number.
//
// Revisions:
//   - 2026-11-02 12:05: initial creation, for the Q13 decision of change C33
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRetention_HeldHandles(t *testing.T) {
	var (
		best = int64(math.MaxInt64)
		each int64
	)

	// The least of a few rounds, because floating garbage from the run can
	// only add to a round, never take from it.
	for range RETENTION_ROUNDS {
		var (
			control = _Retained(
				t,
				0,
			)
			held = _Retained(
				t,
				1,
			)
		)

		each = (held - control) / RETENTION_HANDLES

		t.Logf(
			"held=%d control=%d bytes per handle=%d",
			held,
			control,
			each,
		)

		best = min(
			best,
			each,
		)
	}

	t.Logf(
		"least bytes per handle=%d",
		best,
	)

	if best > RETENTION_LIMIT {
		t.Fatalf(
			"a kept handle holds %d bytes, want at most %d",
			best,
			RETENTION_LIMIT,
		)
	}
}
