package graph_test

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

// _Said is what running a script said: everything it printed, and how it ended.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
type _Said struct {
	printed string
	failure string
}

// _Run compiles a script and runs it, capturing what it printed through the
// printer the runtime carries on its context.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: collects print through WithPrinter rather than by
//     swapping the process's standard error
func _Run(t *testing.T, path string, src []byte) *_Said {
	t.Helper()

	var (
		guard   sync.Mutex
		printed []string
	)

	ctx := runtime.WithPrinter(t.Context(), func(msg string) {
		guard.Lock()
		defer guard.Unlock()

		printed = append(printed, msg)
	})

	art, err := runtime.NewCompiler().Compile(path, src)

	var failure string

	if err == nil {
		_, err = art.Run(ctx)
	}

	if err != nil {
		failure = _Unplaced(err.Error())
	}

	guard.Lock()
	defer guard.Unlock()

	return &_Said{printed: strings.Join(printed, "\n"), failure: failure}
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
func TestRoundTrip_EverySampleDerivesRegeneratesAndBehaves(t *testing.T) {
	refused := make(map[string]bool)

	for _, name := range _Entries(t) {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(SAMPLES + name)
			if err != nil {
				t.Fatal(err)
			}

			report, err := graph.Of(src, SAMPLES+name, nil)
			if err != nil {
				// A refusal is an answer, not a skip: it is asserted against
				// the known set below, and here for its sentinel.
				if !errors.Is(err, graph.ErrConstant) {
					t.Fatalf("refused for an unexpected reason: %v", err)
				}

				refused[name] = true

				return
			}

			if err := graph.Check(report.Graph); err != nil {
				t.Fatalf("check: %v", err)
			}

			out, err := graph.Emit(report.Graph)
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
				t.Fatalf("printed differs\n--- was\n%s\n--- now\n%s\n--- generated\n%s",
					was.printed, now.printed, out)
			}

			if was.failure != now.failure {
				t.Fatalf("ended differently\n was: %s\n now: %s\n--- generated\n%s",
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
				t.Fatalf("a second pass differs\n--- once\n%s\n--- twice\n%s", once, twice)
			}
		})
	}
}

// _Emitted is the script a script's graph generates.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Emitted(t *testing.T, src []byte, path string) []byte {
	t.Helper()

	report, err := graph.Of(src, path, nil)
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Emit(report.Graph)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

// TestRoundTrip_DepthIsReportedNotAssumed is the second number.
//
// Behaviour is the gate and this is the report: how much of each workflow the
// graph actually models, rather than carries as a body it cannot see into.
// The two are never one, because the POC counted "derivation did not complain"
// as fidelity twice and got a high number that meant nothing.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestRoundTrip_DepthIsReportedNotAssumed(t *testing.T) {
	modelled := 0

	for _, name := range _Carried(t) {
		report := _Sample(t, name)

		steps := len(report.Graph.GetThreads()[0].GetStatic().GetSteps())
		if steps > 0 {
			modelled++
		}

		t.Logf("%-18s threads=%2d steps=%2d functions=%2d unknown=%v",
			name, len(report.Graph.GetThreads()), steps,
			len(report.Graph.GetFunctions()), report.Unknown)
	}

	t.Logf("modelled: %d of %d carried, %d entries in all",
		modelled, len(_Carried(t)), len(_Entries(t)))

	if modelled < 5 {
		t.Fatalf("want at least the five workflows modelled, got %d", modelled)
	}
}

// TestRoundTrip_ALibraryIsNotAnEntry records that the one sample defining no
// entry point is refused for that reason rather than derived empty.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func TestRoundTrip_ALibraryIsNotAnEntry(t *testing.T) {
	src, err := os.ReadFile(SAMPLES + LIBRARY)
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Of(src, SAMPLES+LIBRARY, nil)
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
