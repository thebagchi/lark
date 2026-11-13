package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	// SAMPLE carries a flow as a comment and the Starlark generated from it.
	//
	// Revisions:
	//   - 2026-09-20 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:06: one more ".." after the move under internal (C91w11)
	//   - 2026-11-10 00:25: moved to tests/, with one more ".." in the path (C120a, Q311)
	SAMPLE = "../../../../../samples/graph.star"

	// MARK opens the commented flow, and GENERATED the code below it.
	//
	// Revisions:
	//   - 2026-09-20 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	MARK      = "# flow:"
	GENERATED = "# Generated from the flow above:"

	// PREFIX is what every commented line of the flow begins with.
	//
	// Revisions:
	//   - 2026-09-20 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PREFIX = "#   "
)

// TestSample_SaysWhatItGenerates keeps a sample honest.
//
// samples/graph.star shows a flow and the code generated from it, and claims
// the code is that generator's output pasted unedited. Nothing but this checks
// that: the two would drift the first time the generator changed, and a sample
// that lies about its own output is worse than no sample.
//
// It reads the flow back out of the comment, generates from it, and compares.
//
// Revisions:
//   - 2026-09-20 21:27: initial creation
//   - 2026-10-02 01:50: reads a flow
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSample_SaysWhatItGenerates(t *testing.T) {
	text, err := os.ReadFile(SAMPLE)
	if err != nil {
		t.Fatal(err)
	}

	commented, code, found := strings.Cut(
		string(text),
		GENERATED,
	)
	if !found {
		t.Fatalf(
			"want %q in the sample",
			GENERATED,
		)
	}

	_, graphed, found := strings.Cut(
		commented,
		MARK,
	)
	if !found {
		t.Fatalf(
			"want %q in the sample",
			MARK,
		)
	}

	var lines []string

	for _, line := range strings.Split(
		graphed,
		"\n",
	) {
		if strings.HasPrefix(
			line,
			PREFIX,
		) || line == "#" {
			lines = append(
				lines,
				strings.TrimPrefix(
					line,
					PREFIX,
				),
			)
		}
	}

	// json.Compact proves the comment is JSON before protojson sees it, so a
	// broken comment fails here rather than as a confusing proto error.
	var tidy bytes.Buffer

	err = json.Compact(
		&tidy,
		[]byte(strings.Join(
			lines,
			"\n",
		)),
	)
	if err != nil {
		t.Fatalf(
			"the commented flow is not json: %v",
			err,
		)
	}

	var declared workflowpb.Flow

	err = protojson.Unmarshal(
		tidy.Bytes(),
		&declared,
	)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(&declared)
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimSpace(string(out)) != strings.TrimSpace(code) {
		t.Fatalf(
			"the sample's code is not what its flow generates\nwant\n%s\ngot\n%s",
			out,
			code,
		)
	}
}
