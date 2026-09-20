package observe

import (
	"context"
	"errors"
	"fmt"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// LANE and OTHER are two thread numbers, and BROKE the text of the one
	// failure in these tests.
	LANE  = 1
	OTHER = 2
	BROKE = "it broke"

	// ANON is what the interpreter calls an anonymous function.
	ANON = "lambda"
)

// TestBlame_IgnoresACancellationHoweverEarly is a property the end-to-end tests
// cannot see reliably.
//
// A thread stopped by fail-fast may report before the thread that failed - the
// two goroutines are independent, and which lands first is timing. A recorder
// that blamed whatever arrived first would then name a cancelled function as
// the cause of a failure, and only on some runs. Planting that produced a test
// that still passed, because the ordering happened to favour the failure.
//
// Asked directly, there is no ordering to be lucky about.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
func TestBlame_IgnoresACancellationHoweverEarly(t *testing.T) {
	into := _NewRecorder()

	stopped := fmt.Errorf("%s: %w", "patient", context.Canceled)

	into.Started(OTHER, "patient", 0)
	into.Ended(OTHER, "patient", stopped)

	into.Started(LANE, "bad", 0)
	into.Ended(LANE, "bad", errors.New(BROKE))

	cause := into._Cause()
	if cause == nil {
		t.Fatal("want the failure blamed")
	}

	if cause.GetFunction() != "bad" {
		t.Fatalf("want the function that failed, got %q", cause.GetFunction())
	}

	if cause.GetThread() != LANE {
		t.Fatalf("want thread %d, got %d", LANE, cause.GetThread())
	}
}

// TestBlame_KeepsTheFirstFailure records the rule for two genuine failures:
// the first heard of, not the last.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
func TestBlame_KeepsTheFirstFailure(t *testing.T) {
	into := _NewRecorder()

	into.Ended(LANE, "first", errors.New(BROKE))
	into.Ended(OTHER, "second", errors.New("also broke"))

	if into._Cause().GetFunction() != "first" {
		t.Fatalf("want the first failure kept, got %q", into._Cause().GetFunction())
	}
}

// TestBecause_StandsInWhenNothingWasBlamed records what a run that failed with
// no node reported says: the run's own text, no function named.
//
// A nil cause on a failed run would read to a host as "nothing went wrong",
// which is the one thing it must not say.
//
// Revisions:
//   - 2026-09-20 11:40: initial creation
func TestBecause_StandsInWhenNothingWasBlamed(t *testing.T) {
	into := _NewRecorder()

	cause := into._Because(workflowpb.Status_STATUS_FAILED, BROKE)
	if cause == nil {
		t.Fatal("want a failed run to carry a cause even with no node blamed")
	}

	if cause.GetFailure() != BROKE {
		t.Fatalf("want the run's own text, got %q", cause.GetFailure())
	}

	if cause.GetFunction() != "" {
		t.Fatalf("want no function named, got %q", cause.GetFunction())
	}

	if into._Because(workflowpb.Status_STATUS_CANCELLED, "") != nil {
		t.Fatal("want no cause for a cancelled run")
	}
}

// TestResolve_NamesALambdaOnlyWhenALaneIsUnambiguous is the rule the naming
// design rests on, and the end-to-end tests cannot reach it.
//
// A fork names a lane and that lane declares one function, so a spawned lambda
// resolves. A wrapper runs on the lane that called it, and a lane may hold
// several — then there is nothing to choose between them, and choosing would be
// right sometimes and silently wrong the rest.
//
// Planting the guard away left every graph test passing, because none of them
// puts two functions on a lane and then reports a lambda there.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
func TestResolve_NamesALambdaOnlyWhenALaneIsUnambiguous(t *testing.T) {
	into := _NewRecorder()

	into._At(LANE, "greet")

	if into._Resolve(LANE, ANON) != "greet" {
		t.Fatalf("want one candidate resolved, got %q", into._Resolve(LANE, ANON))
	}

	// A second function on the same lane, which is what a wrapper does.
	into._At(LANE, "farewell")

	if into._Resolve(LANE, ANON) != "" {
		t.Fatalf("want two candidates left unnamed, got %q", into._Resolve(LANE, ANON))
	}

	// A lane with nothing placed cannot name anything either.
	if into._Resolve(OTHER, ANON) != "" {
		t.Fatalf("want an empty lane to name nothing, got %q", into._Resolve(OTHER, ANON))
	}

	// A real name is never replaced, however many candidates share its lane.
	if into._Resolve(LANE, "greet") != "greet" {
		t.Fatal("want a named function left alone")
	}
}
