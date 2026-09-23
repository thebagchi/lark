// Why: both of these read what a caller cannot. One checks the shape of an
// unexported struct, and the other reads how a run ended without going through
// the error a caller is given.
package observe

import (
	"context"
	"reflect"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// TestExecution_HoldsNoContext enforces for this package what nothing else
// enforces anywhere.
//
// review.md's phase 3 defends a context living on a _Run by arguing that a
// _Run is unreachable once the call that made it returns, and warns that
// anything storing one past that call silently breaks the argument. A run
// handed back to a caller is exactly such a thing, so no field of it is a
// context and none is anything the scheduler owns.
//
// Revisions:
//   - 2026-09-20 01:38: initial creation, as TestEntry_HoldsNoContext
//   - 2026-09-23 23:28: reads the run a caller now holds, the entry a store
//     held having gone with the store
func TestExecution_HoldsNoContext(t *testing.T) {
	kind := reflect.TypeOf((*context.Context)(nil)).Elem()
	held := reflect.TypeOf(Execution{})

	for i := range held.NumField() {
		field := held.Field(i)

		if field.Type == kind {
			t.Fatalf("%s is a context, which this package must not hold", field.Name)
		}

		if field.Type.Implements(kind) {
			t.Fatalf("%s implements context.Context", field.Name)
		}
	}
}

// TestStart_SurvivesSomethingThatIsNotAnArtifact proves the goroutine is
// guarded.
//
// A nil artifact panics where it is dereferenced, on a goroutine of this
// package's own - and a panic there cannot be recovered from outside it, so
// without a guard this takes the host down. Invoke guards the script's own
// evaluation, and everything before it was unguarded, which is the failure
// mode arriving through a door starting a run opened.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
//   - 2026-09-23 23:28: reads the run's own ending, there being no store to
//     look an entry up in
func TestStart_SurvivesSomethingThatIsNotAnArtifact(t *testing.T) {
	run := Start(t.Context(), nil)

	_, err := run.Wait()
	if err == nil {
		t.Fatal("want the run to have failed")
	}

	if _Became(run.err) != workflowpb.Status_STATUS_FAILED {
		t.Fatalf("want it reported failed, got %v", _Became(run.err))
	}
}
