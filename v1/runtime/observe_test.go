// This file tests what the facade adds for a run that outlives the call
// starting it. Unlike runtime_test.go it does not import only the facade: one
// test names observe's own Start to show the two are one call.
package runtime_test

import (
	"bytes"
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
	var run = runtime.Start(
		t.Context(),
		_Compiled(
			t,
			HOST_FIXTURE,
		),
	)

	begun := run.Status().GetStatus()

	if begun != workflowpb.Status_STATUS_RUNNING &&
		begun != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want a run under way or done, got %v",
			begun,
		)
	}

	got, err := run.Wait()
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"want %s, got %s",
			EXPECTED_TOTAL,
			got,
		)
	}

	if run.Status().GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want succeeded, got %v",
			run.Status().GetStatus(),
		)
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
	built := _Compiled(
		t,
		HOST_FIXTURE,
	)

	var (
		through = runtime.Start(
			t.Context(),
			built,
		)
		direct = observe.Start(
			t.Context(),
			built,
		)
	)

	for _, run := range []*runtime.Execution{through, direct} {
		got, err := run.Wait()
		if err != nil {
			t.Fatal(err)
		}

		if got.String() != EXPECTED_TOTAL {
			t.Fatalf(
				"want %s, got %s",
				EXPECTED_TOTAL,
				got,
			)
		}
	}
}

// TestFacade_EvaluatesThroughOneImport checks a host gets a run's value, its
// graph and its prints from one call of the facade, with the same value Start
// and Wait give.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func TestFacade_EvaluatesThroughOneImport(t *testing.T) {
	result, err := runtime.Evaluate(
		t.Context(),
		_Compiled(
			t,
			HOST_FIXTURE,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Value.String() != EXPECTED_TOTAL {
		t.Fatalf(
			"want %s, got %s",
			EXPECTED_TOTAL,
			result.Value,
		)
	}

	if result.Graph.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf(
			"want succeeded, got %v",
			result.Graph.GetStatus(),
		)
	}

	spoken, err := runtime.Evaluate(
		t.Context(),
		_Compiled(
			t,
			SPEAKING_FIXTURE,
		),
		runtime.WithLog(new(bytes.Buffer)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(spoken.Prints) != SPOKEN {
		t.Fatalf(
			"want %d prints, got %d",
			SPOKEN,
			len(spoken.Prints),
		)
	}
}
