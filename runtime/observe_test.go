// This file tests the four calls phase 4 adds to the facade. Unlike
// runtime_test.go it does not import only the facade, and cannot: two of these
// tests are about the relationship between the facade's store and a store of
// your own, which needs both names.
package runtime_test

import (
	"errors"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/observe"
)

// UNCALLED is a function the host fixture does not define, so nothing can run
// it and a graph declaring it is the only way it appears.
const UNCALLED = "never-run"

// TestFacade_StartsPollsAndWaitsThroughOneImport is what phase 4 exists for: a
// host writes no registration, holds no store, and starts a script.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestFacade_StartsPollsAndWaitsThroughOneImport(t *testing.T) {
	id := runtime.Start(t.Context(), _Compiled(t, HOST_FIXTURE))

	snap, err := runtime.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	begun := snap.GetStatus() == workflowpb.Status_STATUS_RUNNING ||
		snap.GetStatus() == workflowpb.Status_STATUS_SUCCEEDED

	if !begun {
		t.Fatalf("want a run under way or done, got %v", snap.GetStatus())
	}

	got, err := runtime.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != EXPECTED_TOTAL {
		t.Fatalf("want %s, got %s", EXPECTED_TOTAL, got)
	}

	snap, err = runtime.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	if snap.GetStatus() != workflowpb.Status_STATUS_SUCCEEDED {
		t.Fatalf("want succeeded, got %v", snap.GetStatus())
	}
}

// TestFacade_SharesOneStoreWithObserve records that the four calls are the
// store's own, not a second store hiding behind them.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestFacade_SharesOneStoreWithObserve(t *testing.T) {
	id := runtime.Start(t.Context(), _Compiled(t, HOST_FIXTURE))

	_, err := runtime.STORE.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Status(id)
	if err != nil {
		t.Fatalf("want the facade to find what its own store started, got %v", err)
	}
}

// TestFacade_LeavesRoomForAStoreOfYourOwn is the escape hatch the package-level
// store is only acceptable because of. If this stops working, the cost of that
// global stops having a remedy.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestFacade_LeavesRoomForAStoreOfYourOwn(t *testing.T) {
	mine := observe.New()

	id := mine.Start(t.Context(), _Compiled(t, HOST_FIXTURE))

	_, err := mine.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Status(id)
	if !errors.Is(err, runtime.ErrUnknown) {
		t.Fatalf("want a separate store to be separate, got %v", err)
	}
}

// TestFacade_ReexportsTheSameSentinel records that ErrUnknown is one value, so
// errors.Is matches whichever name a host reaches it by.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation
func TestFacade_ReexportsTheSameSentinel(t *testing.T) {
	_, err := runtime.Status(MISSING_RUN)

	if !errors.Is(err, observe.ErrUnknown) {
		t.Fatalf("want observe's own sentinel, got %v", err)
	}

	if !errors.Is(err, runtime.ErrUnknown) {
		t.Fatalf("want the facade's name for it to match too, got %v", err)
	}
}

// TestFacade_SuppliesAGraphWithoutLeavingIt is sub-phase 4.1: everything phase
// 7 added was reachable only by abandoning the facade and holding a store.
//
// It names no option type, which is the other half of phase 7's decision not to
// alias one - runtime.Option is already artifact.Option, so a host has to be
// able to pass this inline without naming it.
//
// Revisions:
//   - 2026-09-20 19:55: initial creation
func TestFacade_SuppliesAGraphWithoutLeavingIt(t *testing.T) {
	declared := &runtime.Graph{
		Functions: []*workflowpb.Function{{Name: UNCALLED}},
	}

	id := runtime.Start(t.Context(), _Compiled(t, HOST_FIXTURE), runtime.WithGraph(declared))

	_, err := runtime.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := runtime.Status(id)
	if err != nil {
		t.Fatal(err)
	}

	for _, lane := range snap.GetThreads() {
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
