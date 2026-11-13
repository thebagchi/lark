package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

const (
	// LATESLOW is the deterministic late-spawn fixture: main sleeps until the
	// failing thread stops the run, so it never reaches its second spawn.
	//
	// Revisions:
	//   - 2026-11-05 08:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: holds the script text, inlined from testdata/lateslow.star (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LATESLOW = `
# The failing thread ends the run while main sleeps, so main never reaches its
# second spawn. The sleep outlasts any test: only the stop wakes main, so the
# order never depends on timing. Compare slowsibling.star, where main reaches
# both spawns before the stop, and which may lose the second to a race.

def patient():
    sleep(30000)

def bad():
    assert(False, "this one breaks")

def main():
    h = spawn(bad)

    sleep(30000)
    join(h, spawn(patient))
`

	// LATE_RUNS is how many times the fixture runs on each side.
	//
	// Revisions:
	//   - 2026-11-05 08:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LATE_RUNS = 50

	// PATIENT, BAD and LATE_FN are functions of the scripts below.
	//
	// Revisions:
	//   - 2026-11-05 08:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PATIENT = "patient"
	BAD     = "bad"
	LATE_FN = "quick"
)

// _Stopper is a plugin whose stop() cancels the context the run was started
// with, as a host does when it stops a run.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Stopper struct {
	cancel context.CancelFunc
}

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Stopper) Name() string {
	return "stopper"
}

// Values supplies stop.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Stopper) Values() starlark.StringDict {
	return starlark.StringDict{
		"stop": starlark.NewBuiltin(
			"stop",
			func(
				*starlark.Thread,
				*starlark.Builtin,
				starlark.Tuple,
				[]starlark.Tuple,
			) (starlark.Value, error) {
				s.cancel()

				return starlark.None, nil
			},
		),
	}
}

// _Both runs text twice with a stop() available: once with the live recorder
// (WithTracking) and once with the ledger (no option). It returns the final
// graph of each.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func _Both(
	t *testing.T,
	text string,
) (*workflowpb.Graph, *workflowpb.Graph) {
	t.Helper()

	var graphs [2]*workflowpb.Graph

	for idx, opts := range [][]common.RunOption{
		{artifact.WithTracking()},
		nil,
	} {
		var (
			ctx, cancel = context.WithCancel(t.Context())
			built, err  = artifact.Compile(
				&common.Source{
					Entry: "unstarted.star",
					Text:  []byte(text),
				},
				artifact.WithPlugins(&_Stopper{
					cancel: cancel,
				}),
			)
		)
		if err != nil {
			cancel()
			t.Fatalf(
				"compile: %v",
				err,
			)
		}

		run, err := observe.Start(
			ctx,
			built,
			append(
				[]common.RunOption{_Quiet()},
				opts...,
			)...,
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = run.Wait()

		cancel()

		graphs[idx] = run.Status()
	}

	return graphs[0], graphs[1]
}

// _Kept is the status of the node named name, and whether the graph has one.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Kept(
	graph *workflowpb.Graph,
	name string,
) (workflowpb.Status, bool) {
	node := _Node(
		graph,
		name,
	)

	return node.GetStatus(), node != nil
}

// TestUnstarted_Shapes runs scripts where a stop keeps a spawn from happening,
// or does not, on both books. Each graph must equal the other, and must hold
// exactly the NOT_STARTED nodes and edges the case lists.
//
// Success: for every case, the recorder's and the ledger's graphs are equal,
// each wanted node has the wanted status, no other node is NOT_STARTED, and
// each wanted edge exists.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestUnstarted_Shapes(t *testing.T) {
	const FAILS = `
def bad():
    assert(False, "this one breaks")

`

	cases := map[string]struct {
		text    string
		status  workflowpb.Status
		cause   string
		missing []string
		edges   [][2]string
	}{
		"late spawn after a failure": {
			text: FAILS + `
def patient():
    sleep(30000)

def main():
    h = spawn(bad)
    sleep(30000)
    join(h, spawn(patient))
`,
			status:  workflowpb.Status_STATUS_FAILED,
			cause:   BAD,
			missing: []string{PATIENT},
			edges:   [][2]string{{"main", PATIENT}},
		},
		"spawn after the caller stopped the run": {
			text: `
def quick():
    return 1

def main():
    stop()
    spawn(quick)
`,
			status:  workflowpb.Status_STATUS_CANCELLED,
			missing: []string{LATE_FN},
			edges:   [][2]string{{"main", LATE_FN}},
		},
		"a spawn in a lambda": {
			text: FAILS + `
def patient():
    sleep(30000)

def main():
    h = spawn(bad)
    sleep(30000)
    join(h, spawn(lambda: patient()))
`,
			status:  workflowpb.Status_STATUS_FAILED,
			cause:   BAD,
			missing: []string{PATIENT},
			edges:   [][2]string{{"main", PATIENT}},
		},
		"a caller that ran and a caller that did not": {
			text: `
def quick():
    return 1

def helper():
    spawn(quick)

def idle():
    spawn(quick)

def main():
    stop()
    spawn(quick)
`,
			status:  workflowpb.Status_STATUS_CANCELLED,
			missing: []string{LATE_FN},
			edges:   [][2]string{{"main", LATE_FN}},
		},
		"a run that succeeded shows no skipped branch": {
			text: `
def never():
    return 1

def main():
    if False:
        spawn(never)
`,
			status: workflowpb.Status_STATUS_SUCCEEDED,
		},
		"a spawn in a function that never ran": {
			text: FAILS + `
def patient():
    sleep(30000)

def idle():
    spawn(patient)

def main():
    join(spawn(bad))
`,
			status: workflowpb.Status_STATUS_FAILED,
			cause:  BAD,
		},
		"the target started, so it is no node to add": {
			text: FAILS + `
def main():
    h = spawn(bad)
    sleep(30000)
    join(h, spawn(bad))
`,
			status: workflowpb.Status_STATUS_FAILED,
			cause:  BAD,
		},
		"a prevented thread's own spawns stay out": {
			text: FAILS + `
def leaf():
    return 1

def patient():
    spawn(leaf)

def main():
    h = spawn(bad)
    sleep(30000)
    join(h, spawn(patient))
`,
			status:  workflowpb.Status_STATUS_FAILED,
			cause:   BAD,
			missing: []string{PATIENT},
			edges:   [][2]string{{"main", PATIENT}},
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				recorded, ledgered := _Both(
					t,
					tc.text,
				)

				_Alike(
					t,
					name,
					recorded,
					ledgered,
				)

				if recorded.GetStatus() != tc.status {
					t.Fatalf(
						"run status %v, want %v",
						recorded.GetStatus(),
						tc.status,
					)
				}

				if recorded.GetCause().GetFunction() != tc.cause {
					t.Fatalf(
						"cause %q, want %q",
						recorded.GetCause().GetFunction(),
						tc.cause,
					)
				}

				_Missing(
					t,
					recorded,
					tc.missing,
					tc.edges,
				)
			},
		)
	}
}

// _Missing fails the test unless the graph's NOT_STARTED nodes are exactly
// missing, and each of edges is a call of the graph.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Missing(
	t *testing.T,
	graph *workflowpb.Graph,
	missing []string,
	edges [][2]string,
) {
	t.Helper()

	var got []string

	for _, node := range graph.GetFunctions() {
		if node.GetStatus() == workflowpb.Status_STATUS_NOT_STARTED {
			got = append(
				got,
				node.GetName(),
			)

			if len(node.GetThreads()) != 0 {
				t.Fatalf(
					"%s: not started but lists threads %v",
					node.GetName(),
					node.GetThreads(),
				)
			}
		}
	}

	if !reflect.DeepEqual(
		got,
		missing,
	) {
		t.Fatalf(
			"not started %v, want %v",
			got,
			missing,
		)
	}

	for _, edge := range edges {
		if !_Called(
			graph,
			edge[0],
			edge[1],
		) {
			t.Fatalf(
				"no edge %s to %s in %v",
				edge[0],
				edge[1],
				graph.GetCalls(),
			)
		}
	}
}

// TestUnstarted_LateSlowIsIdentical runs the late-spawn fixture LATE_RUNS times
// on the recorder and on the ledger. Every run must give the same graph: the
// failing thread and main's sleeping stop make the order certain.
//
// Success: all 2*LATE_RUNS graphs are equal to the first, which has patient
// NOT_STARTED, an edge main to patient, and the cause bad. Command:
// go test -race -count=1 -run TestUnstarted_LateSlowIsIdentical ./v1/runtime/observe/
// (and with -count=50 for the repeat the change asks for).
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func TestUnstarted_LateSlowIsIdentical(t *testing.T) {
	var (
		built = _Compile(
			t,
			LATESLOW,
		)
		first *workflowpb.Graph
	)

	for run := range LATE_RUNS {
		for _, opts := range [][]common.RunOption{
			{artifact.WithTracking()},
			nil,
		} {
			started, err := observe.Start(
				t.Context(),
				built,
				append(
					[]common.RunOption{_Quiet()},
					opts...,
				)...,
			)
			if err != nil {
				t.Fatal(err)
			}

			_, _ = started.Wait()

			graph := started.Status()

			if first == nil {
				first = graph

				status, found := _Kept(
					graph,
					PATIENT,
				)
				if !found || status != workflowpb.Status_STATUS_NOT_STARTED {
					t.Fatalf(
						"patient is %v (found %v), want NOT_STARTED in %v",
						status,
						found,
						graph,
					)
				}

				if !_Called(
					graph,
					ENTRY,
					PATIENT,
				) || graph.GetCause().GetFunction() != BAD {
					t.Fatalf(
						"no edge to patient, or the cause is not bad: %v",
						graph,
					)
				}
			}

			if !proto.Equal(
				first,
				graph,
			) {
				t.Fatalf(
					"run %d: graph differs\nfirst: %v\nthis:  %v",
					run,
					first,
					graph,
				)
			}
		}
	}
}

// TestUnstarted_BundleGivesTheSameGraph saves the fixture, loads the bundle and
// runs the loaded artifact. The graph must equal the compiled artifact's, so a
// bundle shows the spawn the stop prevented too.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestUnstarted_BundleGivesTheSameGraph(t *testing.T) {
	var (
		built = _Compile(
			t,
			LATESLOW,
		)
		saved, err = built.Save()
	)
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	loaded, err := artifact.Load(saved)
	if err != nil {
		t.Fatalf(
			"load: %v",
			err,
		)
	}

	var graphs []*workflowpb.Graph

	for _, art := range []*artifact.Artifact{
		built,
		loaded,
	} {
		run, _ := _Waited(
			t,
			art,
		)

		graphs = append(
			graphs,
			run.Status(),
		)
	}

	_Alike(
		t,
		"compiled and loaded",
		graphs[0],
		graphs[1],
	)

	if status, _ := _Kept(
		graphs[1],
		PATIENT,
	); status != workflowpb.Status_STATUS_NOT_STARTED {
		t.Fatalf(
			"loaded: patient is %v, want NOT_STARTED",
			status,
		)
	}
}

// TestUnstarted_ChangesPatchInTheNodes checks what a user interface holds: the
// changes of a watched run, applied to {}, give the final graph with the
// NOT_STARTED node and its edge.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestUnstarted_ChangesPatchInTheNodes(t *testing.T) {
	var (
		guard   sync.Mutex
		changes []*workflowpb.Change
	)

	run, err := observe.Start(
		t.Context(),
		_Compile(
			t,
			LATESLOW,
		),
		_Quiet(),
		artifact.WithChanges(func(change *workflowpb.Change) {
			guard.Lock()
			defer guard.Unlock()

			changes = append(
				changes,
				change,
			)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = run.Wait()

	guard.Lock()
	defer guard.Unlock()

	var held any = map[string]any{}

	for _, change := range changes {
		held = _Folded(
			t,
			held,
			change,
		)
	}

	want := _Decoded(
		t,
		run.Status(),
	)
	if !reflect.DeepEqual(
		held,
		want,
	) {
		raw, _ := json.Marshal(held)

		t.Fatalf(
			"patched into\n%s\nwant\n%v",
			raw,
			want,
		)
	}

	if status, _ := _Kept(
		run.Status(),
		PATIENT,
	); status != workflowpb.Status_STATUS_NOT_STARTED {
		t.Fatalf(
			"patient is %v, want NOT_STARTED",
			status,
		)
	}
}
