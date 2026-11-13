package tests

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// FINISHED is the graph the spec prints for a run of EVERY, as JSON.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:25: holds the JSON text, inlined from testdata/every_graph.json (C114,
	//     Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FINISHED = `
{
  "status": "STATUS_SUCCEEDED",
  "functions": [
    {"name": "attempt", "status": "STATUS_SUCCEEDED"},
    {"name": "bounded", "status": "STATUS_SUCCEEDED"},
    {"name": "main", "status": "STATUS_SUCCEEDED"},
    {"name": "once", "status": "STATUS_SUCCEEDED"},
    {"name": "sign", "status": "STATUS_SUCCEEDED"},
    {"name": "slow", "status": "STATUS_SUCCEEDED"},
    {"name": "task_a", "status": "STATUS_SUCCEEDED"},
    {"name": "task_b", "status": "STATUS_SUCCEEDED"},
    {"name": "task_c", "status": "STATUS_SUCCEEDED"},
    {"name": "task_d", "status": "STATUS_SUCCEEDED"},
    {"name": "task_e", "status": "STATUS_SUCCEEDED"},
    {"name": "work", "status": "STATUS_SUCCEEDED"}
  ],
  "calls": [
    {"caller": "main", "callee": "attempt"},
    {"caller": "main", "callee": "bounded"},
    {"caller": "main", "callee": "once"},
    {"caller": "main", "callee": "sign"},
    {"caller": "main", "callee": "slow"},
    {"caller": "main", "callee": "task_a"},
    {"caller": "main", "callee": "task_b"},
    {"caller": "main", "callee": "task_c"},
    {"caller": "main", "callee": "task_d"},
    {"caller": "main", "callee": "task_e"},
    {"caller": "main", "callee": "work"},
    {"caller": "task_a", "callee": "sign"},
    {"caller": "task_a", "callee": "work"}
  ]
}
`

	// PORT is what the run supplies the fixture's one argument without a
	// default with.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PORT = 8080
)

// TestEvery_FinishesWithTheGraphTheSpecPrints runs the every-construct fixture
// and compares the graph it finishes with to the one the spec prints: every
// function and its status, and every call between them.
//
// Exact, though four threads nothing joins may end either way: each is an
// older call of work, whose node shows its newest call, the spawn main joins
// last.
//
// Revisions:
//   - 2026-10-02 01:54: initial creation
//   - 2026-10-02 13:12: supplies its argument as JSON
//   - 2026-10-02 15:34: a call graph, compared exactly, there being no thread
//     to settle
//   - 2026-10-02 16:08: hands the run its argument, as an option of it
//   - 2026-10-02 16:18: builds the argument as a Struct
//   - 2026-10-03 16:31: hands the argument as a map, which the run builds a Struct from
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:25: reads EVERY and FINISHED as text, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEvery_FinishesWithTheGraphTheSpecPrints(t *testing.T) {
	built, err := runtime.Compile(&common.Source{
		Entry: "every.star",
		Text:  []byte(_Text(EVERY)),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
		runtime.WithArgs(map[string]any{"port": PORT}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	var (
		got  = run.Status()
		want = new(workflowpb.Graph)
	)

	err = protojson.Unmarshal(
		[]byte(_Text(FINISHED)),
		want,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(
		got,
		want,
	) {
		shown := protojson.MarshalOptions{
			Multiline: true,
		}

		t.Fatalf(
			"finished with\n%s\nwant\n%s",
			shown.Format(got),
			shown.Format(want),
		)
	}
}
