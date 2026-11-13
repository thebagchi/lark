package tests

import (
	"fmt"
	"testing"
	"time"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// STRESS_SPAWNS is how many threads each stress script starts.
	//
	// Revisions:
	//   - 2026-11-05 18:53: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	STRESS_SPAWNS = 300

	// STRESS_RUNS is how many times each stress case runs.
	//
	// Revisions:
	//   - 2026-11-05 18:53: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	STRESS_RUNS = 20

	// STRESS_LIMIT is how long one run may take before the test calls it hung.
	//
	// Revisions:
	//   - 2026-11-05 18:53: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	STRESS_LIMIT = 30 * time.Second
)

// TestSpawnStress_NoLostRetire starts many threads that end together, with
// and without a stop part way, and checks the ledger lost none of them.
//
// A retire that was lost or done twice shows in one of three ways: the run
// never ends, a function stays RUNNING though every thread ended, or the
// ledger's graph differs from the recorder's when no stop was made.
//
// Success: Wait returns inside STRESS_LIMIT on every run; no node is RUNNING at
// the end; with no stop the two graphs are equal and "work" and "leaf" are
// SUCCEEDED with an edge main -> work and work -> leaf; with a stop the node
// "work" is present and the run is CANCELLED.
//
// Revisions:
//   - 2026-11-05 08:14: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSpawnStress_NoLostRetire(t *testing.T) {
	const (
		WORK = `
def leaf():
    return 1

def work():
    join(spawn(leaf))
    return leaf()

`
	)

	cases := map[string]struct {
		text    string
		stopped bool
	}{
		"all end": {
			text: WORK + fmt.Sprintf(
				`
def main():
    hs = [spawn(work) for i in range(%d)]
    join(*hs)
`, STRESS_SPAWNS,
			),
		},
		"stop part way": {
			text: WORK + fmt.Sprintf(
				`
def main():
    hs = []
    for i in range(%d):
        if i == %d:
            stop()
        hs.append(spawn(work))
    join(*hs)
`, STRESS_SPAWNS,
				STRESS_SPAWNS/4,
			),
			stopped: true,
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				for range STRESS_RUNS {
					recorded, ledgered := _Limited(
						t,
						tc.text,
					)

					_NoneRunning(
						t,
						ledgered,
					)

					if tc.stopped {
						_Cancelled(
							t,
							ledgered,
						)

						continue
					}

					_Alike(
						t,
						name,
						recorded,
						ledgered,
					)
					_Whole(
						t,
						ledgered,
					)
				}
			},
		)
	}
}

// _Pair is the graph each book drew for one run.
//
// Revisions:
//   - 2026-11-05 08:30: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Pair struct {
	recorded *workflowpb.Graph
	ledgered *workflowpb.Graph
}

// _Limited runs text on both books and fails the test when it takes longer
// than STRESS_LIMIT.
//
// Revisions:
//   - 2026-11-05 08:30: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Limited(
	t *testing.T,
	text string,
) (*workflowpb.Graph, *workflowpb.Graph) {
	t.Helper()

	done := make(
		chan _Pair,
		1,
	)

	go func() {
		recorded, ledgered := _Both(
			t,
			text,
		)
		done <- _Pair{
			recorded: recorded,
			ledgered: ledgered,
		}
	}()

	select {
	case got := <-done:
		return got.recorded, got.ledgered
	case <-time.After(STRESS_LIMIT):
		t.Fatalf(
			"a run did not end in %v",
			STRESS_LIMIT,
		)
	}

	return nil, nil
}

// _NoneRunning fails the test when a node of graph is still RUNNING.
//
// Revisions:
//   - 2026-11-05 08:30: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _NoneRunning(
	t *testing.T,
	graph *workflowpb.Graph,
) {
	t.Helper()

	for _, node := range graph.GetFunctions() {
		if node.GetStatus() == workflowpb.Status_STATUS_RUNNING {
			t.Fatalf(
				"node %q still RUNNING",
				node.GetName(),
			)
		}
	}
}

// _Cancelled fails the test unless graph is of a cancelled run.
//
// Revisions:
//   - 2026-11-05 08:30: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Cancelled(
	t *testing.T,
	graph *workflowpb.Graph,
) {
	t.Helper()

	if graph.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf(
			"status %v, want CANCELLED",
			graph.GetStatus(),
		)
	}
}

// _Whole fails the test unless work and leaf both succeeded and both edges of
// the script are in graph.
//
// Revisions:
//   - 2026-11-05 08:30: initial creation (C53)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Whole(
	t *testing.T,
	graph *workflowpb.Graph,
) {
	t.Helper()

	for _, fn := range []string{"work", "leaf"} {
		status, found := _Kept(
			graph,
			fn,
		)
		if !found || status != workflowpb.Status_STATUS_SUCCEEDED {
			t.Fatalf(
				"%s is %v (found %v)",
				fn,
				status,
				found,
			)
		}
	}

	if !_Called(
		graph,
		"main",
		"work",
	) || !_Called(
		graph,
		"work",
		"leaf",
	) {
		t.Fatal("an edge is missing")
	}
}
