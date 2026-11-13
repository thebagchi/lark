package observe

import (
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Book is what a run keeps its graph in: the scheduler's Reporter, which is
// how the run tells the book anything, and the steps Start, Eval and
// _Perform take around the run.
//
// Two books implement it, and _Opened picks between them, so no flag lives
// inside either. The Recorder is the live graph: it folds every report under
// one lock and tells a watcher each change, so a host can ask for the graph at
// any moment. The Ledger folds each thread's reports on that thread alone and
// joins them at the end, so a report takes no lock the other threads wait
// for. Both draw the same graph at the end of the same run.
//
// Unstarted is told, once and before Finished, the spawns written in the
// script of a run that did not succeed. Each spawn written in a function the
// run called, of a function it did not, becomes a node NOT_STARTED and an edge
// from the function that wrote it.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation, from the research POC q1_endgraph
//   - 2026-11-05 00:27: _Unstarted, for the spawns a stop prevented (C45, Q18)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: renamed the methods _Began, _Unstarted, _Finished, _Because, _Drawn, as a
//     test in tests/ needs them (C120a, Q314)
//   - 2026-11-10 22:40: uses Eval, formerly Evaluate (C121, Q350)
type _Book interface {
	common.Reporter
	Began()
	Unstarted(sites []common.Spawn)
	Finished(
		status workflowpb.Status,
		cause *workflowpb.Cause,
	)
	Because(
		status workflowpb.Status,
		failure string,
	) *workflowpb.Cause
	_Fault() error
	Drawn() *workflowpb.Graph
	_Standing() *workflowpb.Graph
}

var (
	_ _Book = (*Recorder)(nil)
	_ _Book = (*Ledger)(nil)
)
