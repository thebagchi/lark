package graph_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

const (
	// SAMPLE carries a graph as a comment and the Starlark generated from it.
	SAMPLE = "../../../samples/graph.star"

	// MARK opens the commented graph, and GENERATED the code below it.
	MARK      = "# graph:"
	GENERATED = "# Generated from the graph above:"

	// COMMENT is what every commented line of the graph begins with.
	COMMENT = "#   "
)

// TestSample_SaysWhatItGenerates keeps a sample honest.
//
// samples/graph.star shows a graph and the code generated from it, and claims
// the code is that generator's output pasted unedited. Nothing but this checks
// that: the two would drift the first time the generator changed, and a sample
// that lies about its own output is worse than no sample.
//
// It reads the graph back out of the comment, generates from it, and compares.
//
// Revisions:
//   - 2026-09-20 21:27: initial creation
func TestSample_SaysWhatItGenerates(t *testing.T) {
	text, err := os.ReadFile(SAMPLE)
	if err != nil {
		t.Fatal(err)
	}

	commented, code, found := strings.Cut(string(text), GENERATED)
	if !found {
		t.Fatalf("want %q in the sample", GENERATED)
	}

	_, graphed, found := strings.Cut(commented, MARK)
	if !found {
		t.Fatalf("want %q in the sample", MARK)
	}

	var lines []string

	for _, line := range strings.Split(graphed, "\n") {
		if strings.HasPrefix(line, COMMENT) || line == "#" {
			lines = append(lines, strings.TrimPrefix(line, COMMENT))
		}
	}

	// json.Compact proves the comment is JSON before protojson sees it, so a
	// broken comment fails here rather than as a confusing proto error.
	var tidy bytes.Buffer

	err = json.Compact(&tidy, []byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("the commented graph is not json: %v", err)
	}

	var declared workflowpb.Graph

	err = protojson.Unmarshal(tidy.Bytes(), &declared)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(&declared)
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(string(out)) != strings.TrimSpace(code) {
		t.Fatalf("the sample's code is not what its graph generates\nwant\n%s\ngot\n%s",
			out, code)
	}
}
