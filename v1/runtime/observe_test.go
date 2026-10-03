// This file tests what the facade adds for a run that outlives the call
// starting it. Unlike runtime_test.go it does not import only the facade: one
// test names observe's own Start to show the two are one call.
package runtime_test

import (
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/observe"
)

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
