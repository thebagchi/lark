package tests

// This file tests what the facade adds for a run that outlives the call
// starting it. Unlike runtime_test.go it does not import only the facade: one
// test names observe's own Start to show the two are one call.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	"bytes"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// TestFacade_StartsWaitsAndAsksThroughOneImport is what the facade exists for:
// a host writes no registration, holds nothing of this package's, and starts a
// script.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as
//     TestFacade_StartsPollsAndWaitsThroughOneImport
//   - 2026-09-23 23:35: holds the run rather than an id into a store
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFacade_StartsWaitsAndAsksThroughOneImport(t *testing.T) {
	var run, err = runtime.Start(
		t.Context(),
		_Compiled(
			t,
			"host.star",
		),
	)
	if err != nil {
		t.Fatal(err)
	}

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
//   - 2026-11-07 23:23: Execution is an interface and Print is in common (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFacade_StartIsObservesOwn(t *testing.T) {
	var (
		built = _Compiled(
			t,
			"host.star",
		)
		through, err = runtime.Start(
			t.Context(),
			built,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	direct, err := observe.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, run := range []common.Execution{through, direct} {
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
//   - 2026-11-09 11:37: reads the script from memory (C114, Q281)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func TestFacade_EvaluatesThroughOneImport(t *testing.T) {
	result, err := runtime.Eval(
		t.Context(),
		_Compiled(
			t,
			"host.star",
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

	spoken, err := runtime.Eval(
		t.Context(),
		_Compiled(
			t,
			"speaking.star",
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
