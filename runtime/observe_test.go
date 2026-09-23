// This file tests what the facade adds for a run that outlives the call
// starting it. Unlike runtime_test.go it does not import only the facade: one
// test names observe's own Start to show the two are one call.
package runtime_test

import (
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/observe"
)

// UNCALLED is a function the host fixture does not define, so nothing can run
// it and a graph declaring it is the only way it appears.
const UNCALLED = "never-run"

// TestFacade_StartsWaitsAndAsksThroughOneImport is what the facade exists for:
// a host writes no registration, holds nothing of this package's, and starts a
// script.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as
//     TestFacade_StartsPollsAndWaitsThroughOneImport
//   - 2026-09-23 23:35: holds the run rather than an id into a store
func TestFacade_StartsWaitsAndAsksThroughOneImport(t *testing.T) {
	run := runtime.Start(t.Context(), _Compiled(t, HOST_FIXTURE))

	begun := run.Status().GetStatus()

	if begun != workflowpb.Status_STATUS_RUNNING &&
		begun != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want a run under way or done, got %v", begun)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != EXPECTED_TOTAL {
		t.Fatalf("want %s, got %s", EXPECTED_TOTAL, got)
	}

	if run.Status().GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want succeeded, got %v", run.Status().GetStatus())
	}
}

// TestFacade_StartIsObservesOwn records that the facade's call is the one
// observe declares, rather than a second way of starting a run that could
// drift from it.
//
// This replaces two tests about the relationship between a package-level store
// and a store of your own. Neither exists: a run is the caller's, so there is
// no global to escape from and no escape hatch to keep working.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as TestFacade_SharesOneStoreWithObserve
//   - 2026-09-23 23:35: there is no store to share, so this checks the two
//     calls produce the same thing
func TestFacade_StartIsObservesOwn(t *testing.T) {
	built := _Compiled(t, HOST_FIXTURE)

	through := runtime.Start(t.Context(), built)
	direct := observe.Start(t.Context(), built)

	for _, run := range []*runtime.Execution{through, direct} {
		got, err := run.Wait()
		if err != nil {
			t.Fatal(err)
		}

		if got.String() != EXPECTED_TOTAL {
			t.Fatalf("want %s, got %s", EXPECTED_TOTAL, got)
		}
	}
}

// TestFacade_SuppliesAGraphWithoutLeavingIt checks that a graph reaches a run
// through the facade, since everything it buys was otherwise reachable only by
// naming another package.
//
// It names no option type, which is deliberate: runtime.Option is already
// artifact.Option, so a host has to be able to pass this inline without naming
// it.
//
// Revisions:
//   - 2026-09-20 19:55: initial creation
//   - 2026-09-23 23:35: asks the run it started
func TestFacade_SuppliesAGraphWithoutLeavingIt(t *testing.T) {
	declared := &runtime.Graph{
		Functions: []*workflowpb.Function{{Name: UNCALLED}},
	}

	run := runtime.Start(t.Context(), _Compiled(t, HOST_FIXTURE), runtime.WithGraph(declared))

	_, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	for _, lane := range run.Status().GetThreads() {
		for _, node := range lane.GetLive().GetNodes() {
			if node.GetFunction() != UNCALLED {
				continue
			}

			if node.GetStatus() != workflowpb.Status_STATUS_PENDING {
				t.Fatalf("want %s pending, got %v", UNCALLED, node.GetStatus())
			}

			return
		}
	}

	t.Fatalf("want %s reported, which is what supplying a graph buys", UNCALLED)
}
