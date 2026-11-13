package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestEmit_AllocationsStayLow guards the cost of the emitter: one Emit of the
// flow sample must stay under a ceiling of allocations. A template call that
// finds a method by reflection (the form before C35b) allocates on each call
// and breaks the ceiling.
//
// Measured at C35b with go test -run AllocationsStay -v: 1410 on the base, 1174
// on this change. The ceiling is the latter with a margin of about 10 percent.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEmit_AllocationsStayLow(t *testing.T) {
	const CEILING = 1290

	text, err := os.ReadFile(filepath.Join(
		SAMPLES,
		"flow.star",
	))
	if err != nil {
		t.Fatal(err)
	}

	flow, err := graph.Of(&common.Source{
		Entry: "flow.star",
		Text:  text,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Emit(flow)
	if err != nil {
		t.Fatal(err)
	}

	got := testing.AllocsPerRun(
		20,
		func() {
			_, err := graph.Emit(flow)
			if err != nil {
				t.Fatal(err)
			}
		},
	)
	t.Logf(
		"allocs per Emit: %v",
		got,
	)

	if got > CEILING {
		t.Fatalf(
			"got %v allocations per Emit, want at most %v",
			got,
			CEILING,
		)
	}
}
