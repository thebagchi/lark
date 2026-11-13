package observe

import (
	"context"
	"log/slog"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// Eval evaluates art's entry point on the caller's goroutine and returns
// once the run is over, with what it returned, its graph and what it printed.
//
// No goroutine is started for the run itself, which is what a caller that
// would only wait for one pays to have one woken. The script's own threads
// still each run on one of their own. It performs a run as Start does, with
// one function, so the two cannot end one run differently.
//
// Cancelling ctx stops the run, and the error is then scheduler.ErrCancelled. A print
// still goes to the logger the options name, exactly as it would, and is kept
// in the result whatever level that logger writes at.
//
// The result is never nil. For a run that failed, its Value is nil and the
// error is the one Wait gives for the same run. For an artifact lark did not
// compile, a nil one included, the error is artifact.ErrArtifact, and the result has
// an empty graph and no prints.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-10-29 01:26: applies opts once, and sets the logger that keeps the
//     prints on what they told, rather than on a copy of opts
//   - 2026-11-07 23:04: takes common.Artifact and common.RunOption, and returns ErrNotCompiled for
//     a nil or foreign artifact (C91w6, Q118)
//   - 2026-11-07 23:23: returns common.Result (C91w7)
//   - 2026-11-08 10:35: sets the tee on given itself, since WithLogger now refuses a
//     logger WithLog made (C75, Q235)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 17:15: uses ErrArtifact, formerly ErrNotCompiled, and its comment names
//     WithLog, since WithLogger is dropped; renamed from Evaluate (C121, Q340,
//     Q348, Q350)
func Eval(
	ctx context.Context,
	art common.Artifact,
	opts ...common.RunOption,
) (*common.Result, error) {
	held, err := artifact.Concrete(art)
	if err != nil {
		return &common.Result{
			Graph: new(workflowpb.Graph),
		}, err
	}

	var (
		given      = artifact.Apply(opts)
		transcript = new(_Transcript)
		tee        = _NewTee(
			given.Logger,
			transcript,
		)
	)

	// After opts, so the run logs through the tee, which logs on to the logger
	// opts named. Set here, not through WithLog, which refuses a second logger.
	given.Logger = slog.New(tee)

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
		held,
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
//   - 2026-11-02 17:49: draws from the run's book, whichever kind it is
//   - 2026-11-07 23:23: returns a common.Result (C91w7)
//   - 2026-11-10 01:00: now uses Drawn, were _Drawn (C120a, Q314)
func _Ended(
	value starlark.Value,
	into _Book,
	transcript *_Transcript,
) *common.Result {
	return &common.Result{
		Value:  value,
		Graph:  into.Drawn(),
		Prints: transcript._Lines(),
	}
}
