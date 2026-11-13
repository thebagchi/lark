package common

import (
	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Execution is one run: how to wait for it, and how it is doing. Stopping it is
// cancelling the context it was started with.
//
// A caller holds this rather than an id, and nothing holds it for them. What
// is worth keeping about a finished run, and for how long, is the host's
// decision - so there is no store here, no identifier to look one up by, and
// nothing that forgets on a schedule of its own.
//
// Only lark starts one: the observe package inside the runtime holds the type
// that implements it (Q117). When Start returns an error the Execution is nil
// and must not be used (Q233).
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: moved from observe, as the interface of the Execution
//     struct; nil with an error, and not to be used then (C91w7, Q101, Q117,
//     Q233)
type Execution interface {
	// Wait blocks until the run is over and returns what it produced.
	Wait() (starlark.Value, error)

	// Done is closed when the run is over.
	Done() <-chan struct{}

	// Status is how this run is doing, or how it ended.
	Status() *workflowpb.Graph

	// Snapshot is the graph as the run holds it now, with the status RUNNING.
	Snapshot() *workflowpb.Graph
}

// Print is one line a run's script printed: Thread, the id of the thread that
// printed it, Function, the function that called print, and Text, the line.
//
// The same three things the run's logger is told as a record, so a host that
// reads them here and one that reads its log see one line the same way.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: moved from observe (C91w7)
type Print struct {
	Thread   string
	Function string
	Text     string
}

// Result is how a run Eval performed ended: Value, what main returned, and
// nil when the run failed; Graph, the run's graph as it ended, the one Status
// gives a started run once Done is closed; and Prints, each line the script
// printed, in the order printed.
//
// All three at the end, for a host that waits for a run and has no use for it
// while it goes. A failed run still has a graph and the lines it printed
// before it failed, which is often the best account of why.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: moved from observe (C91w7)
//   - 2026-11-10 17:15: names Eval, formerly Evaluate, in the text (C121, Q338)
type Result struct {
	Value  starlark.Value
	Graph  *workflowpb.Graph
	Prints []*Print
}
