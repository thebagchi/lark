package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
)

const (
	// EVERY is the every-construct fixture, and FINISHED the graph the spec
	// prints for a run of it.
	EVERY    = "testdata/every.star"
	FINISHED = "testdata/every_graph.json"

	// PORT is what the run supplies the fixture's one argument without a
	// default with.
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
func TestEvery_FinishesWithTheGraphTheSpecPrints(t *testing.T) {
	src, err := os.ReadFile(EVERY)
	if err != nil {
		t.Fatal(err)
	}

	built, err := runtime.Compile(&runtime.Source{Entry: filepath.Base(EVERY), Text: src})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	run := runtime.Start(t.Context(), built, runtime.WithArgs(map[string]any{"port": PORT}))

	_, err = run.Wait()
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	got := run.Status()

	want := new(workflowpb.Graph)

	text, err := os.ReadFile(FINISHED)
	if err != nil {
		t.Fatal(err)
	}

	err = protojson.Unmarshal(text, want)
	if err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(got, want) {
		shown := protojson.MarshalOptions{Multiline: true}

		t.Fatalf("finished with\n%s\nwant\n%s", shown.Format(got), shown.Format(want))
	}
}
