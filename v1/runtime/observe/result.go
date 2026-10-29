package observe

import (
	"context"
	"log/slog"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
)

// Result is how a run Evaluate performed ended: Value, what main returned, and
// nil when the run failed; Graph, the run's graph as it ended, the one Status
// gives a started run once Done is closed; and Prints, each line the script
// printed, in the order printed.
//
// All three at the end, for a host that waits for a run and has no use for it
// while it goes. A failed run still has a graph and the lines it printed
// before it failed, which is often the best account of why.
type Result struct {
	Value  starlark.Value
	Graph  *workflowpb.Graph
	Prints []*Print
}

// Evaluate evaluates art's entry point on the caller's goroutine and returns
// once the run is over, with what it returned, its graph and what it printed.
//
// No goroutine is started for the run itself, which is what a caller that
// would only wait for one pays to have one woken. The script's own threads
// still each run on one of their own. It performs a run as Start does, with
// one function, so the two cannot end one run differently.
//
// Cancelling ctx stops the run, and the error is then ErrCancelled. A print
// still goes to the logger the options name, exactly as it would, and is kept
// in the result whatever level that logger writes at.
//
// The result is never nil. For a run that failed, its Value is nil and the
// error is the one Wait gives for the same run.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-10-29 01:26: applies opts once, and sets the logger that keeps the
//     prints on what they told, rather than on a copy of opts
func Evaluate(
	ctx context.Context,
	art *artifact.Artifact,
	opts ...artifact.RunOption,
) (*Result, error) {
	var (
		given      = artifact.Apply(opts)
		transcript = new(_Transcript)
		tee        = _NewTee(
			given.Logger(),
			transcript,
		)
	)

	// After opts, so the run logs through the tee, which logs on to the logger
	// opts named.
	artifact.WithLogger(slog.New(tee))(given)

	into, fault := _Opened(given)
	if fault != nil {
		return _Ended(
			nil,
			into,
			transcript,
		), fault
	}

	value, err := _Perform(
		ctx,
		art,
		into,
		given,
	)
	if err != nil {
		value = nil
	}

	return _Ended(
		value,
		into,
		transcript,
	), err
}

// _Ended is the result of a run that returned value, drawn from into and with
// the lines kept in transcript.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func _Ended(
	value starlark.Value,
	into *_Recorder,
	transcript *_Transcript,
) *Result {
	return &Result{
		Value:  value,
		Graph:  into._Drawn(),
		Prints: transcript._Lines(),
	}
}
