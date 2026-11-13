package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

const (
	// SAMPLES is where the samples of the repository are, from this package.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 23:23: one directory deeper (C91w7)
	//   - 2026-11-10 00:25: moved to tests/, with one more ".." in the path (C120a, Q311)
	SAMPLES = "../../../../../samples/*.star"

	// SEEDS is how many random programs the paired test runs, and EVENT_SEEDS
	// how many random report sequences.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SEEDS       = 100
	EVENT_SEEDS = 3000

	// FUNNEL is how many threads the merge test spawns, and FUNNEL_ROUNDS how
	// many times it runs them.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FUNNEL        = 400
	FUNNEL_ROUNDS = 5

	// FUNNEL_TEXT is a script that spawns FUNNEL threads over three functions,
	// each thread calling a leaf in a loop, so threads end early and in every
	// order.
	//
	// Revisions:
	//   - 2026-11-02 20:50: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FUNNEL_TEXT = `
def leaf(i):
    return i

def left():
    for i in range(20):
        leaf(i)

def right():
    for i in range(20):
        leaf(i)

def main():
    hs = []
    for i in range(%d):
        if i %% 2 == 0:
            hs.append(spawn(left))
        else:
            hs.append(spawn(right))
    join(*hs)
`
)

// _Quiet is the option that sends a run's prints nowhere.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: writes to io.Discard through WithLog, WithLogger being dropped (C121,
//     Q340)
func _Quiet() common.RunOption {
	return artifact.WithLog(io.Discard)
}

// _Built compiles text as the script entry names, with every plugin of the
// runtime, as a host does.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Built(
	t *testing.T,
	entry string,
	text string,
) common.Artifact {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: entry,
		Text:  []byte(text),
	})
	if err != nil {
		t.Fatalf(
			"%s: compile: %v",
			entry,
			err,
		)
	}

	return built
}

// _Alike fails the test when the two graphs are not equal, and prints both.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Alike(
	t *testing.T,
	label string,
	left *workflowpb.Graph,
	right *workflowpb.Graph,
) {
	t.Helper()

	if proto.Equal(
		left,
		right,
	) {
		return
	}

	t.Fatalf(
		"%s: graphs differ\nfirst:\n%s\nsecond:\n%s",
		label,
		protojson.Format(left),
		protojson.Format(right),
	)
}

// _Waited starts art with opts, waits for it, and returns the run and how it
// ended.
//
// Revisions:
//   - 2026-11-02 18:01: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Waited(
	t *testing.T,
	art common.Artifact,
	opts ...common.RunOption,
) (common.Execution, error) {
	t.Helper()

	var (
		all = append(
			[]common.RunOption{_Quiet()},
			opts...,
		)
	)

	run, err := observe.Start(
		t.Context(),
		art,
		all...,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()

	return run, err
}

// _Samples is the path of each sample script.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Samples(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob(SAMPLES)
	if err != nil || len(files) == 0 {
		t.Fatalf(
			"no samples: %v",
			err,
		)
	}

	return files
}

// _Sample compiles the sample at path.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sample(
	t *testing.T,
	path string,
) common.Artifact {
	t.Helper()

	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return _Built(
		t,
		path,
		string(text),
	)
}

// TestLedger_SamplesPaired checks that the ledger draws the graph the recorder
// draws, for each sample, when both are told the same reports in one order.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLedger_SamplesPaired(t *testing.T) {
	for _, path := range _Samples(t) {
		t.Run(
			filepath.Base(path),
			func(t *testing.T) {
				recorded, ledgered, _ := RunPaired(
					t.Context(),
					_Sample(
						t,
						path,
					),
				)

				_Alike(
					t,
					path,
					recorded,
					ledgered,
				)
			},
		)
	}
}

// RULES holds a script for each rule of the final graph that needs an order
// across threads, or that is easy to get wrong.
//
// Revisions:
//   - 2026-11-02 20:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	RULES = map[string]string{
		// The inner call is f's newest, so f shows its success though the outer
		// calls failed.
		"newest_call_wins": `
def f(n):
    if n == 0:
        return 1

    f(n - 1)
    assert(False, "outer " + str(n))

def main():
    f(2)
`,
		// Two threads call f; the one that started last ends first, with success.
		"newest_call_across_threads": `
def f(wait):
    sleep(wait)

    if wait == 30:
        assert(False, "slow one fails")

def slow():
    f(30)

def fast():
    sleep(5)
    f(1)

def main():
    a = spawn(slow)
    b = spawn(fast)
    join(a, b)
`,
		"recursion": `
def down(n):
    if n > 0:
        down(n - 1)

def main():
    down(4)
`,
		"lambda_spawn": `
def leaf():
    return 1

def main():
    h = spawn(lambda: leaf())
    join(h)
`,
		"nested_spawn": `
def leaf():
    return 1

def mid():
    h = spawn(leaf)
    join(h)

def main():
    a = spawn(mid)
    b = spawn(mid)
    join(a, b)
`,
		"failure_in_thread": `
def bad():
    assert(False, "boom")

def good():
    return 1

def main():
    a = spawn(good)
    b = spawn(bad)
    join(a, b)
`,
		"failure_propagates_up": `
def inner():
    assert(False, "deep")

def middle():
    inner()

def outer():
    middle()

def main():
    outer()
`,
		"retry_repeat_timeout": `
def step():
    return 1

def main():
    repeat(3, step)
    timeout(1000, step)
    retry(2, step)
`,
		"retry_gives_up": `
def flaky():
    assert(False, "again")

def main():
    retry(3, flaky)
`,
		"cancel": `
def spin():
    t = 0
    for i in range(100000000):
        t += i

def main():
    h = spawn(spin)
    cancel(h)
    join(h)
`,
		"unjoined_spawn_cancelled_at_end": `
def spin():
    t = 0
    for i in range(100000000):
        t += i

def quick():
    return 1

def main():
    spawn(spin)
    quick()
`,
		"many_threads": `
def leaf(i):
    return i

def work():
    for i in range(50):
        leaf(i)

def main():
    hs = []
    for _ in range(32):
        hs.append(spawn(work))
    join(*hs)
`,
	}
)

// TestLedger_RulesPaired checks each script of RULES told to both books.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLedger_RulesPaired(t *testing.T) {
	for name, text := range RULES {
		t.Run(
			name,
			func(t *testing.T) {
				recorded, ledgered, _ := RunPaired(
					t.Context(),
					_Built(
						t,
						name+".star",
						text,
					),
				)

				_Alike(
					t,
					name,
					recorded,
					ledgered,
				)
			},
		)
	}
}

// _Program writes a random acyclic program: eight functions that call, spawn,
// repeat, retry, time out and sometimes fail, and a main that spawns threads.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation, from the research POC q1_endgraph
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Program(seed uint64) string {
	const FUNCS = 8

	var (
		rng = rand.New(rand.NewPCG(
			seed,
			seed*7+1,
		))
		text strings.Builder
	)

	for i := FUNCS - 1; i >= 0; i-- {
		fmt.Fprintf(
			&text,
			"def f%d():\n",
			i,
		)

		for range 1 + rng.IntN(4) {
			j := i + 1 + rng.IntN(FUNCS)
			if j >= FUNCS {
				text.WriteString("    return 1\n")

				break
			}

			switch rng.IntN(9) {
			case 0:
				fmt.Fprintf(
					&text,
					"    h = spawn(f%d)\n    join(h)\n",
					j,
				)
			case 1:
				fmt.Fprintf(
					&text,
					"    h = spawn(lambda: f%d())\n    join(h)\n",
					j,
				)
			case 2:
				fmt.Fprintf(
					&text,
					"    repeat(2, f%d)\n",
					j,
				)
			case 3:
				fmt.Fprintf(
					&text,
					"    timeout(2000, f%d)\n",
					j,
				)
			case 4:
				fmt.Fprintf(
					&text,
					"    retry(2, f%d)\n",
					j,
				)
			case 5:
				fmt.Fprintf(
					&text,
					"    spawn(f%d)\n",
					j,
				)
			default:
				fmt.Fprintf(
					&text,
					"    f%d()\n",
					j,
				)
			}
		}

		if rng.IntN(8) == 0 {
			fmt.Fprintf(
				&text,
				"    assert(False, \"seed %d f%d\")\n",
				seed,
				i,
			)
		}
	}

	text.WriteString("\ndef main():\n    hs = []\n")

	for range 2 + rng.IntN(6) {
		fmt.Fprintf(
			&text,
			"    hs.append(spawn(f%d))\n",
			rng.IntN(FUNCS),
		)
	}

	fmt.Fprintf(
		&text,
		"    f%d()\n    join(*hs)\n",
		rng.IntN(FUNCS),
	)

	return text.String()
}

// TestLedger_RandomPaired checks random programs told to both books.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLedger_RandomPaired(t *testing.T) {
	var causes int

	for seed := uint64(1); seed <= SEEDS; seed++ {
		var (
			text                  = _Program(seed)
			recorded, ledgered, _ = RunPaired(
				t.Context(),
				_Built(
					t,
					fmt.Sprintf(
						"seed%d.star",
						seed,
					),
					text,
				),
			)
		)

		_Alike(
			t,
			fmt.Sprintf(
				"seed %d\n%s",
				seed,
				text,
			),
			recorded,
			ledgered,
		)

		if recorded.GetCause() != nil {
			causes++
		}
	}

	t.Logf(
		"%d of %d programs failed with a cause",
		causes,
		SEEDS,
	)
}

// _Reports makes the random sequence of reports that seed names: ends with no
// start, retries on open calls, spawns onto lanes that exist, calls left open.
//
// Revisions:
//   - 2026-11-02 18:00: initial creation, lifted from TestLedger_EventsPaired
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Reports(seed uint64) func(into common.Reporter) {
	var (
		rng = rand.New(rand.NewPCG(
			seed,
			99,
		))
		first = errors.New("base one")
		other = errors.New("base two")
		wrap  = fmt.Errorf(
			"wrapped: %w",
			first,
		)
		errs = []error{nil, nil, first, other, wrap, context.Canceled}
	)

	return func(into common.Reporter) {
		for range 10 + rng.IntN(60) {
			var (
				thread = fmt.Sprintf(
					"t%d",
					rng.IntN(5),
				)
				name = fmt.Sprintf(
					"f%d",
					rng.IntN(5),
				)
			)

			switch rng.IntN(6) {
			case 0, 1:
				into.Started(
					thread,
					&common.Line{
						Name: name,
					},
				)
			case 2:
				into.Started(
					thread,
					&common.Line{
						Name: name,
						Child: fmt.Sprintf(
							"t%d",
							rng.IntN(5),
						),
						Builtin: "spawn",
					},
				)
			case 3:
				into.Started(
					thread,
					&common.Line{
						Name:    name,
						Attempt: int32(1 + rng.IntN(3)),
					},
				)
			default:
				into.Ended(
					thread,
					name,
					errs[rng.IntN(len(errs))],
				)
			}
		}
	}
}

// TestLedger_EventsPaired tells both books random sequences of reports that a
// real run does not make. It covers the threads a node lists at the end, and
// their order.
//
// Revisions:
//   - 2026-11-02 18:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLedger_EventsPaired(t *testing.T) {
	var open int

	for seed := uint64(1); seed <= EVENT_SEEDS; seed++ {
		recorded, ledgered := Drive(
			workflowpb.Status_STATUS_FAILED,
			_Reports(seed),
		)

		_Alike(
			t,
			fmt.Sprintf(
				"seed %d",
				seed,
			),
			recorded,
			ledgered,
		)

		for _, node := range recorded.GetFunctions() {
			if len(node.GetThreads()) > 0 {
				open++

				break
			}
		}
	}

	if open == 0 {
		t.Fatal("no sequence ended inside a call: thread lists untested")
	}
}

// TestLedger_BlamesTheFunctionTheRecorderBlames checks the cause of a run that
// failed, in the default run and in the live run.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func TestLedger_BlamesTheFunctionTheRecorderBlames(t *testing.T) {
	tests := []struct {
		name     string
		function string
	}{
		{
			"failure_in_thread",
			"bad",
		},
		{
			"failure_propagates_up",
			"inner",
		},
		{
			"retry_gives_up",
			"flaky",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				art := _Built(
					t,
					test.name+".star",
					RULES[test.name],
				)

				for _, opts := range [][]common.RunOption{
					nil,
					{artifact.WithTracking()},
				} {
					run, err := _Waited(
						t,
						art,
						opts...,
					)
					if err == nil {
						t.Fatalf(
							"options %d: want the run to fail",
							len(opts),
						)
					}

					got := run.Status().GetCause().GetFunction()
					if got != test.function {
						t.Fatalf(
							"options %d: cause names %q, want %q",
							len(opts),
							got,
							test.function,
						)
					}
				}
			},
		)
	}
}

// TestTracking_SamplesEndAlike checks that a run ends with the same graph
// with the live graph and without it, for each sample.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph; renamed from
//     TestLiveGraph_SamplesEndAlike (C121, Q350)
func TestTracking_SamplesEndAlike(t *testing.T) {
	for _, path := range _Samples(t) {
		t.Run(
			filepath.Base(path),
			func(t *testing.T) {
				art := _Sample(
					t,
					path,
				)

				var (
					live, liveErr = _Waited(
						t,
						art,
						artifact.WithTracking(),
					)
					plain, plainErr = _Waited(
						t,
						art,
					)
				)

				if fmt.Sprint(liveErr) != fmt.Sprint(plainErr) {
					t.Fatalf(
						"errors differ: %v and %v",
						liveErr,
						plainErr,
					)
				}

				_Alike(
					t,
					path,
					live.Status(),
					plain.Status(),
				)
			},
		)
	}
}

// TestTracking_EvalEndsAlike checks the graph Eval hands back is the
// same with the live graph and without it.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: renamed from TestLiveGraph_EvaluateEndsAlike; uses Eval and
//     WithTracking, formerly Evaluate and WithLiveGraph (C121, Q350)
func TestTracking_EvalEndsAlike(t *testing.T) {
	art := _Built(
		t,
		"many.star",
		RULES["many_threads"],
	)

	var (
		live, liveErr = observe.Eval(
			t.Context(),
			art,
			_Quiet(),
			artifact.WithTracking(),
		)
		plain, plainErr = observe.Eval(
			t.Context(),
			art,
			_Quiet(),
		)
	)

	if liveErr != nil || plainErr != nil {
		t.Fatalf(
			"evaluate failed: %v and %v",
			liveErr,
			plainErr,
		)
	}

	_Alike(
		t,
		"evaluate",
		live.Graph,
		plain.Graph,
	)
}

// TestStatus_DefaultRunIsEmptyUntilTheEnd checks that a run with no live graph
// answers an empty running graph while it goes, and its graph at the end.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStatus_DefaultRunIsEmptyUntilTheEnd(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run, err := observe.Start(
		ctx,
		_Compile(
			t,
			SLOW,
		),
		_Quiet(),
	)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(SETTLE)

	want := &workflowpb.Graph{
		Status: workflowpb.Status_STATUS_RUNNING,
	}

	_Alike(
		t,
		"mid-run",
		want,
		run.Status(),
	)

	stop()

	_, _ = run.Wait()

	ended := run.Status()
	if ended.GetStatus() != workflowpb.Status_STATUS_CANCELLED {
		t.Fatalf(
			"want cancelled at the end, got %v",
			ended.GetStatus(),
		)
	}

	if len(ended.GetFunctions()) == 0 {
		t.Fatal("want the functions of the run at the end")
	}
}

// TestStatus_TrackingAnswersWhileTheRunGoes checks that WithTracking keeps
// Status answering with the functions called so far.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph; renamed from
//     TestStatus_LiveGraphAnswersWhileTheRunGoes (C121, Q350)
func TestStatus_TrackingAnswersWhileTheRunGoes(t *testing.T) {
	ctx, stop := context.WithCancel(t.Context())
	defer stop()

	run, err := observe.Start(
		ctx,
		_Compile(
			t,
			SLOW,
		),
		_Quiet(),
		artifact.WithTracking(),
	)
	if err != nil {
		t.Fatal(err)
	}

	var (
		deadline = time.Now().Add(PATIENCE)
	)

	for time.Now().Before(deadline) {
		node := _Node(
			run.Status(),
			"slow",
		)
		if node != nil {
			if node.GetStatus() != workflowpb.Status_STATUS_RUNNING {
				t.Fatalf(
					"want slow running, got %v",
					node.GetStatus(),
				)
			}

			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("the live graph never showed slow while it ran")
}

// TestStatus_ChangesTurnTrackingOn checks that a run with a watcher keeps
// the live graph without being asked.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:25: renamed from TestStatus_ChangesTurnTheLiveGraphOn (C121, Q350)
func TestStatus_ChangesTurnTrackingOn(t *testing.T) {
	var told atomic.Int64

	run, err := _Waited(
		t,
		_Compile(
			t,
			QUICK,
		),
		artifact.WithChanges(func(*workflowpb.Change) {
			told.Add(1)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if told.Load() == 0 {
		t.Fatal("want the watcher told the changes of the run")
	}

	if _Node(
		run.Status(),
		"main",
	) == nil {
		t.Fatal("want the graph of the run")
	}
}

// TestLedger_MergesThreadsThatEndEarly checks that the folds of many threads
// that end in any order join into the graph a live run draws, and that the
// graph holds exactly the functions and calls the script makes. Run with
// -race.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 22:40: uses WithTracking, formerly WithLiveGraph (C121, Q350)
func TestLedger_MergesThreadsThatEndEarly(t *testing.T) {
	art := _Built(
		t,
		"funnel.star",
		fmt.Sprintf(
			FUNNEL_TEXT,
			FUNNEL,
		),
	)

	var (
		wantNodes = []string{"leaf", "left", "main", "right"}
		wantEdges = []string{"left>leaf", "main>left", "main>right", "right>leaf"}
	)

	for round := range FUNNEL_ROUNDS {
		var (
			plain, plainErr = _Waited(
				t,
				art,
			)
			live, liveErr = _Waited(
				t,
				art,
				artifact.WithTracking(),
			)
		)

		if plainErr != nil || liveErr != nil {
			t.Fatalf(
				"round %d: %v and %v",
				round,
				plainErr,
				liveErr,
			)
		}

		ended := plain.Status()

		_Alike(
			t,
			"funnel",
			live.Status(),
			ended,
		)

		var gotNodes, gotEdges []string

		for _, node := range ended.GetFunctions() {
			gotNodes = append(
				gotNodes,
				node.GetName(),
			)

			if node.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
				t.Fatalf(
					"%s is %v, want succeeded",
					node.GetName(),
					node.GetStatus(),
				)
			}
		}

		for _, edge := range ended.GetCalls() {
			gotEdges = append(
				gotEdges,
				edge.GetCaller()+">"+edge.GetCallee(),
			)
		}

		if fmt.Sprint(gotNodes) != fmt.Sprint(wantNodes) {
			t.Fatalf(
				"nodes %v, want %v",
				gotNodes,
				wantNodes,
			)
		}

		if fmt.Sprint(gotEdges) != fmt.Sprint(wantEdges) {
			t.Fatalf(
				"edges %v, want %v",
				gotEdges,
				wantEdges,
			)
		}
	}
}

// TestStatus_PollingWhileThreadsEnd asks a run with no live graph for its
// status from many goroutines while threads start and end. Its worth is under
// -race: Status must read nothing a thread is writing.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestStatus_PollingWhileThreadsEnd(t *testing.T) {
	art := _Built(
		t,
		"funnel.star",
		fmt.Sprintf(
			FUNNEL_TEXT,
			FUNNEL,
		),
	)

	for range FUNNEL_ROUNDS {
		run, err := observe.Start(
			t.Context(),
			art,
			_Quiet(),
		)
		if err != nil {
			t.Fatal(err)
		}

		var (
			asked sync.WaitGroup
		)

		for range POLLS {
			asked.Add(1)

			go func() {
				defer asked.Done()

				for {
					select {
					case <-run.Done():
						return
					default:
						_ = run.Status()
					}
				}
			}()
		}

		_, err = run.Wait()
		if err != nil {
			t.Fatal(err)
		}

		asked.Wait()
	}
}

// TestSnapshot_WhileThreadsRun checks that Snapshot is safe while 400 threads
// report, and honest about what it is. A goroutine asks for the snapshot over
// and over while the funnel script runs; under -race any read of a lane
// without its lock fails the test. Each snapshot has the status RUNNING and
// only functions the script has; Status of the same run, without the live
// graph, stays the empty running graph until the end; and after the end
// Snapshot and Status give the same finished graph. Run with -race.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSnapshot_WhileThreadsRun(t *testing.T) {
	var (
		art = _Built(
			t,
			"funnel.star",
			fmt.Sprintf(
				FUNNEL_TEXT,
				FUNNEL,
			),
		)
		known = map[string]bool{
			"leaf":  true,
			"left":  true,
			"main":  true,
			"right": true,
		}
	)

	for round := range FUNNEL_ROUNDS {
		run, err := observe.Start(
			t.Context(),
			art,
			_Quiet(),
		)
		if err != nil {
			t.Fatal(err)
		}

		var (
			asked    int
			problems []string
		)

		for !_Closed(run.Done()) {
			snap := run.Snapshot()
			asked++

			problems = append(
				problems,
				_Checked(
					snap,
					known,
				)...,
			)

			got := run.Status()
			if got.GetStatus() == workflowpb.Status_STATUS_RUNNING &&
				len(got.GetFunctions()) != 0 {
				problems = append(
					problems,
					"Status showed functions of a run with no live graph",
				)
			}
		}

		_, err = run.Wait()
		if err != nil {
			t.Fatalf(
				"round %d: %v",
				round,
				err,
			)
		}

		if len(problems) != 0 {
			t.Fatalf(
				"round %d, %d snapshots: %v",
				round,
				asked,
				problems[0],
			)
		}

		_Alike(
			t,
			"after the end",
			run.Snapshot(),
			run.Status(),
		)
	}
}

// _Closed is true when done is closed.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Closed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true

	default:
		return false
	}
}

// _Checked lists what is wrong with a snapshot of a running funnel.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Checked(
	snap *workflowpb.Graph,
	known map[string]bool,
) []string {
	var wrong []string

	if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING &&
		snap.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		wrong = append(
			wrong,
			fmt.Sprintf(
				"status %v",
				snap.GetStatus(),
			),
		)
	}

	for _, node := range snap.GetFunctions() {
		if !known[node.GetName()] {
			wrong = append(
				wrong,
				"unknown function "+node.GetName(),
			)
		}
	}

	return wrong
}

// TestSnapshot_ShowsTheCallsSoFar checks the content of a snapshot: a run that
// sleeps inside helper, called from main, has both functions RUNNING and the
// call between them, while Status of the same run, which keeps no live graph,
// is still the empty running graph.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSnapshot_ShowsTheCallsSoFar(t *testing.T) {
	var (
		art = _Built(
			t,
			"sleeps.star",
			"def helper():\n    sleep(400)\n\ndef main():\n    helper()\n",
		)
	)

	run, err := observe.Start(
		t.Context(),
		art,
		_Quiet(),
	)
	if err != nil {
		t.Fatal(err)
	}

	var (
		snap *workflowpb.Graph
	)

	for range 100 {
		time.Sleep(20 * time.Millisecond)

		snap = run.Snapshot()
		if len(snap.GetFunctions()) == 2 {
			break
		}
	}

	if snap.GetStatus() != workflowpb.Status_STATUS_RUNNING ||
		len(snap.GetFunctions()) != 2 || len(snap.GetCalls()) != 1 {
		t.Fatalf(
			"snapshot %v, want two running functions and one call",
			snap,
		)
	}

	for _, node := range snap.GetFunctions() {
		if node.GetStatus() != workflowpb.Status_STATUS_RUNNING {
			t.Fatalf(
				"%s is %v, want running",
				node.GetName(),
				node.GetStatus(),
			)
		}
	}

	if len(run.Status().GetFunctions()) != 0 {
		t.Fatal("Status of a run with no live graph showed functions mid-run")
	}

	_, err = run.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
