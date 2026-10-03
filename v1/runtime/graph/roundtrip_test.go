package graph_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/script"
)

const (
	// SAMPLES is where the scripts these tests derive live.
	SAMPLES = "../../../samples/"

	// LIBRARY is the one sample that defines no entry point.
	LIBRARY = "strings.star"

	// MODELLED is the fewest samples whose main a flow models as statements,
	// measured 2026-10-02 at four: cancel, failfast, failkinds and graph. It
	// was five while the graph modelled threads; concurrent.star nests its
	// spawns inside a join and prints, and neither is a statement a flow can
	// state.
	MODELLED = 4
)

// _Said is what running a script said: everything it printed, and how it ended.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
type _Said struct {
	printed string
	failure string
}

// _Run compiles a script and runs it, capturing what it printed from the
// transcript the runtime writes.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: collects print through WithPrinter rather than by
//     swapping the process's standard error
//   - 2026-10-02 13:12: reads the transcript, keeping what each line said
//   - 2026-10-02 16:21: reads the records the run's logger writes
func _Run(t *testing.T, path string, src []byte) *_Said {
	t.Helper()

	var out bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&out, nil))

	art, err := runtime.Compile(&runtime.Source{Entry: path, Text: src})

	var failure string

	if err == nil {
		_, err = runtime.Start(t.Context(), art, runtime.WithLogger(logger)).Wait()
	}

	if err != nil {
		failure = _Unplaced(err.Error())
	}

	return &_Said{printed: strings.Join(_Printed(t, out.String()), "\n"), failure: failure}
}

// _Printed is what each record a JSON handler wrote said, without when, the
// thread or the function.
//
// Those are dropped because the two runs compared are one program, not one
// layout: the clock moves between them, and a generated script need not print
// from the functions its source printed from.
//
// Revisions:
//   - 2026-10-02 13:12: initial creation
//   - 2026-10-02 16:21: reads JSON records, a logger's rather than a
//     transcript's lines
func _Printed(t *testing.T, logged string) []string {
	t.Helper()

	var said []string

	for line := range strings.Lines(logged) {
		record := map[string]any{}

		err := json.Unmarshal([]byte(line), &record)
		if err != nil {
			t.Fatalf("record %q: %v", line, err)
		}

		said = append(said, fmt.Sprint(record[slog.MessageKey]))
	}

	return said
}

// _Unplaced is a failure without the position it happened at.
//
// A generated script is the same program at different line numbers, so
// comparing positions compares layout rather than behaviour. Everything after
// the position is what the run actually said.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Unplaced(failure string) string {
	for _, held := range []string{".star:"} {
		at := strings.Index(failure, held)
		if at < 0 {
			continue
		}

		rest := failure[at+len(held):]

		colon := strings.Index(rest, ": ")
		if colon >= 0 {
			return failure[:at] + rest[colon+2:]
		}
	}

	return failure
}

// TestRoundTrip_EverySampleDerivesRegeneratesAndBehaves is the increment,
// measured.
//
// Derive, check, generate, compile, run - and the run says what the original
// said. Behaviour is the gate: anything less is a broken migration.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 01:51: derives a flow through a Source
func TestRoundTrip_EverySampleDerivesRegeneratesAndBehaves(t *testing.T) {
	refused := make(map[string]bool)

	for _, name := range _Entries(t) {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(SAMPLES + name)
			if err != nil {
				t.Fatal(err)
			}

			flow, err := graph.Of(&script.Source{Entry: SAMPLES + name, Text: src})
			if err != nil {
				// A refusal is an answer, not a skip: it is asserted against
				// the known set below, and here for its sentinel.
				if !errors.Is(err, graph.ERR_CONSTANT) {
					t.Fatalf("refused for an unexpected reason: %v", err)
				}

				refused[name] = true

				return
			}

			if err := graph.Check(flow); err != nil {
				t.Fatalf("check: %v", err)
			}

			out, err := graph.Emit(flow)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}

			was := _Run(t, SAMPLES+name, src)
			now := _Run(t, SAMPLES+name, out)

			// Both sides must actually have run. A comparison of two silences
			// is what made this measurement wrong twice in the POC.
			if was.printed == "" && was.failure == "" {
				t.Fatal("the original produced nothing, so nothing was compared")
			}

			if was.printed != now.printed {
				t.Fatalf("printed differs\n--- was\n%s\n--- now\n%s\n"+
					"--- generated\n%s",
					was.printed, now.printed, out)
			}

			if was.failure != now.failure {
				t.Fatalf("ended differently\n was: %s\n now: %s\n--- generated\n"+
					"%s",
					was.failure, now.failure, out)
			}
		})
	}

	_Refused(t, refused)
}

// _Refused checks the set of samples derivation will not carry against the set
// it is known not to carry.
//
// A list rather than a count, and asserted exactly, so that fixing one or
// breaking another both show up here. Skipping what fails is only honest if
// what is skipped is named.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Refused(t *testing.T, refused map[string]bool) {
	t.Helper()

	// Both open with a module-level dict of several pairs. A
	// google.protobuf.Struct is a map and a map has no order, while Starlark
	// prints a dict in the order it was written - so carrying one would give a
	// script that prints something the original did not.
	want := map[string]bool{"patch.star": true, "pointers.star": true}

	for name := range want {
		if !refused[name] {
			t.Fatalf("%s derives now; the known-refused list is out of date", name)
		}
	}

	for name := range refused {
		if !want[name] {
			t.Fatalf("%s stopped deriving and is not a known refusal", name)
		}
	}
}

// TestRoundTrip_ASecondPassChangesNothing records that derivation is stable:
// what it generates derives back to the same thing.
//
// A graph that drifted on the second pass would mean a user interface could
// not round-trip its own edits.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestRoundTrip_ASecondPassChangesNothing(t *testing.T) {
	for _, name := range _Entries(t) {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(SAMPLES + name)
			if err != nil {
				t.Fatal(err)
			}

			if _Uncarried(name) {
				// Refused rather than derived, which the round trip asserts
				// by sentinel; there is no second pass to compare.
				return
			}

			once := _Emitted(t, src, SAMPLES+name)
			twice := _Emitted(t, once, SAMPLES+name)

			if string(once) != string(twice) {
				t.Fatalf(
					"a second pass differs\n--- once\n%s\n--- twice\n%s",
					once,
					twice,
				)
			}
		})
	}
}

// _Emitted is the script a script's flow generates.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 01:51: derives a flow through a Source
func _Emitted(t *testing.T, src []byte, path string) []byte {
	t.Helper()

	flow, err := graph.Of(&script.Source{Entry: path, Text: src})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(flow)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

// TestRoundTrip_DepthIsReportedNotAssumed is the second number.
//
// Behaviour is the gate and this is the report: how much of each workflow the
// flow actually models, rather than carries as text it cannot see into. The
// two are never one, because the POC counted "derivation did not complain" as
// fidelity twice and got a high number that meant nothing. A workflow is
// modelled when its main is statements.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 01:51: counts a main of statements, and functions modelled
//     as statements against those kept as text
func TestRoundTrip_DepthIsReportedNotAssumed(t *testing.T) {
	modelled := 0

	for _, name := range _Carried(t) {
		flow := _Sample(t, name)

		spine := len(flow.GetMain().GetStatement())
		if spine > 0 {
			modelled++
		}

		stated := 0

		for _, fn := range flow.GetFunctions() {
			if fn.GetStatements() != nil {
				stated++
			}
		}

		t.Logf("%-18s main=%2d functions=%2d stated=%2d",
			name, spine, len(flow.GetFunctions()), stated)
	}

	t.Logf("modelled: %d of %d carried, %d entries in all",
		modelled, len(_Carried(t)), len(_Entries(t)))

	if modelled < MODELLED {
		t.Fatalf("want at least %d workflows modelled, got %d", MODELLED, modelled)
	}
}

// TestRoundTrip_ALibraryIsNotAnEntry records that the one sample defining no
// entry point is refused for that reason rather than derived empty.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 01:51: derives through a Source
func TestRoundTrip_ALibraryIsNotAnEntry(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + LIBRARY)
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Of(&script.Source{Entry: SAMPLES + LIBRARY, Text: src})
	if err == nil {
		t.Fatal("want a library refused")
	}

	if !strings.Contains(err.Error(), "entry point") {
		t.Fatalf("want the reason, got %v", err)
	}
}

// _Uncarried reports whether a sample is one derivation refuses, which
// _Refused keeps honest.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Uncarried(name string) bool {
	return name == "patch.star" || name == "pointers.star"
}

// _Sample is the flow one of the shipped samples yields.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-10-02 01:51: derives through a Source
//   - 2026-10-03 00:16: the flow itself, which Of returns with nothing beside it
func _Sample(t *testing.T, name string) *workflowpb.Flow {
	t.Helper()

	src, err := os.ReadFile(SAMPLES + name)
	if err != nil {
		t.Fatal(err)
	}

	flow, err := graph.Of(&script.Source{Entry: SAMPLES + name, Text: src})
	if err != nil {
		t.Fatal(err)
	}

	return flow
}

// _Entries is every sample that defines an entry point, which is every one but
// the library.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Entries(t *testing.T) []string {
	t.Helper()

	found, err := filepath.Glob(SAMPLES + "*.star")
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, path := range found {
		name := filepath.Base(path)
		if name == LIBRARY {
			continue
		}

		names = append(names, name)
	}

	return names
}

// _Carried is every sample derivation will carry, which is every entry but the
// two whose module-level dict the schema cannot order.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Carried(t *testing.T) []string {
	t.Helper()

	var names []string

	for _, name := range _Entries(t) {
		if _Uncarried(name) {
			continue
		}

		names = append(names, name)
	}

	return names
}
