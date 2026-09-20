package graph_test

import (
	"errors"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/graph"
)

// TestDistinct_AcceptsTodayGraph is the happy path Distinct has to keep:
// first, second, main, each once.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
func TestDistinct_AcceptsTodayGraph(t *testing.T) {
	src := &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: "first"},
			{Name: "second"},
			{Name: "main"},
		},
	}

	if err := graph.Distinct(src); err != nil {
		t.Fatal(err)
	}
}

// TestDistinct_NamesTheSecondGreet is why Distinct returns an error
// rather than a bool: a UI that sent two greet bodies needs the name
// in the refusal.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
func TestDistinct_NamesTheSecondGreet(t *testing.T) {
	src := &workflowpb.Graph{
		Functions: []*workflowpb.Function{
			{Name: "greet"},
			{Name: "greet"},
		},
	}

	err := graph.Distinct(src)
	if !errors.Is(err, graph.ErrDuplicate) {
		t.Fatalf("got %v, want ErrDuplicate", err)
	}

	if !strings.Contains(err.Error(), "greet") {
		t.Fatalf("want the name in the error, got %v", err)
	}
}

// TestDistinct_IgnoresNodes is the snapshot case: two Nodes named first
// are not two Functions named first. Distinct reads the Graph only.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
func TestDistinct_IgnoresNodes(t *testing.T) {
	src := &workflowpb.Graph{
		Functions: []*workflowpb.Function{{Name: "first"}},
	}

	snap := &workflowpb.Workflow{
		Status: workflowpb.Status_STATUS_RUNNING,
		Threads: []*workflowpb.Thread{
			{
				Index: 1,
				Nodes: []*workflowpb.Node{{
					Function: "first",
					Status:   workflowpb.Status_STATUS_SUCCEEDED,
				}},
			},
			{
				Index: 3,
				Nodes: []*workflowpb.Node{{
					Function: "first",
					Status:   workflowpb.Status_STATUS_RUNNING,
				}},
			},
		},
	}

	if err := graph.Distinct(src); err != nil {
		t.Fatalf("want one function on two threads accepted, got %v", err)
	}

	count := 0

	for _, lane := range snap.GetThreads() {
		for _, node := range lane.GetNodes() {
			if node.GetFunction() != "first" {
				continue
			}

			count++
		}
	}

	if count != 2 {
		t.Fatalf("want first on two live threads, got %d", count)
	}
}
