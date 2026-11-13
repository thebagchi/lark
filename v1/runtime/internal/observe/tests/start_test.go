// This file tests how a run ends when Start gets something that is not an
// artifact. TestStart_SurvivesSomethingThatIsNotAnArtifact reads how the run
// ended without going through the error a caller is given: it reads the Err
// field of Execution and calls Became. (TestExecution_HoldsNoContext is in
// execution_test.go: reflect reads the field types of the exported Execution.)
//
// Revisions:
//   - 2026-09-23 22:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:49: HoldsNoContext moved to the external package (C65a)
//   - 2026-11-10 00:25: moved to tests/ from execution_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// TestStart_SurvivesSomethingThatIsNotAnArtifact proves the goroutine is
// guarded.
//
// A nil *Artifact, which Start accepts and a nil interface it refuses with
// artifact.ErrArtifact (Q118), panics where it is dereferenced, on a goroutine of this
// package's own - and a panic there cannot be recovered from outside it, so
// without a guard this takes the host down. The run guards the script's own
// evaluation, and everything before it was unguarded, which is the failure
// mode arriving through a door starting a run opened.
//
// Revisions:
//   - 2026-09-20 11:56: initial creation
//   - 2026-09-23 23:28: reads the run's own ending, there being no store to
//     look an entry up in
//   - 2026-11-07 23:04: passes a nil *Artifact, since a nil interface is ErrNotCompiled now
//     (C91w6)
//   - 2026-11-07 23:23: asserts the run to the concrete Execution to read err (C91w7)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrArtifact, formerly ErrNotCompiled (C121, Q348)
func TestStart_SurvivesSomethingThatIsNotAnArtifact(t *testing.T) {
	var none *artifact.Artifact

	run, err := observe.Start(
		t.Context(),
		none,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err == nil {
		t.Fatal("want the run to have failed")
	}

	if observe.Became(run.(*observe.Execution).Err) != workflowpb.Status_STATUS_FAILED {
		t.Fatalf(
			"want it reported failed, got %v",
			observe.Became(run.(*observe.Execution).Err),
		)
	}
}
