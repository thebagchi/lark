package tests

// This file holds _Twin, _Paired, RunPaired and Drive. They feed one run's
// reports to a Recorder and a Ledger, so tests can compare the two graphs.
//
// Revisions:
//   - 2026-11-10 00:25: moved to tests/ from export_test.go (C120a, Q311)

import (
	"context"
	"io"
	"sync"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// _Twin tells one run's reports to a Recorder and to a Ledger, one report at
// a time under one lock, so both books see the reports in one order. With one
// order, the two graphs must be equal whatever the threads did.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation, from the research POC q1_endgraph
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Twin struct {
	mu       sync.Mutex
	recorder *observe.Recorder
	ledger   *observe.Ledger
}

// Started tells both books that thread began the call line names.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Twin) Started(
	thread string,
	line *common.Line,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recorder.Started(
		thread,
		line,
	)
	p.ledger.Started(
		thread,
		line,
	)
}

// Ended tells both books that thread's newest call to name ended with err.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Twin) Ended(
	thread string,
	name string,
	err error,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recorder.Ended(
		thread,
		name,
		err,
	)
	p.ledger.Ended(
		thread,
		name,
		err,
	)
}

// _Finish ends the run in both books and returns the graph each draws.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Twin) _Finish(
	status workflowpb.Status,
	failure string,
) (*workflowpb.Graph, *workflowpb.Graph) {
	p.recorder.Finished(
		status,
		p.recorder.Because(
			status,
			failure,
		),
	)
	p.ledger.Finished(
		status,
		p.ledger.Because(
			status,
			failure,
		),
	)

	return p.recorder.Drawn(), p.ledger.Drawn()
}

// _Paired starts a twin, with both books begun.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Paired() *_Twin {
	twin := &_Twin{
		recorder: observe.NewRecorder(),
		ledger:   observe.NewLedger(),
	}

	twin.recorder.Began()
	twin.ledger.Began()

	return twin
}

// RunPaired evaluates art once, telling its reports to both books, and returns
// the graph the recorder drew at the end and the graph the ledger drew.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-05 00:27: tells both books the spawns a stop may have prevented,
//     as a run's end does (C45)
//   - 2026-11-07 23:04: takes common.Artifact (C91w6)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: writes to io.Discard through WithLog, WithLogger being dropped (C121,
//     Q340)
func RunPaired(
	ctx context.Context,
	built common.Artifact,
) (*workflowpb.Graph, *workflowpb.Graph, error) {
	art, fault := artifact.Concrete(built)
	if fault != nil {
		return nil, nil, fault
	}

	var (
		given = artifact.Apply(nil)
		twin  = _Paired()
	)

	artifact.WithLog(io.Discard)(given)
	artifact.Reporting(twin)(given)

	_, err := artifact.Perform(
		ctx,
		art,
		given,
	)

	var (
		status = observe.Became(err)
		failed = observe.Why(err)
	)

	sites := observe.Written(
		art,
		status,
	)

	twin.recorder.Unstarted(sites)
	twin.ledger.Unstarted(sites)

	recorded, ledgered := twin._Finish(
		status,
		failed,
	)

	return recorded, ledgered, err
}

// Drive tells one sequence of reports to both books and returns the graph each
// draws, with the run left as the sequence leaves it: calls may stay open.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func Drive(
	status workflowpb.Status,
	events func(into common.Reporter),
) (*workflowpb.Graph, *workflowpb.Graph) {
	twin := _Paired()

	events(twin)

	return twin._Finish(
		status,
		"run text",
	)
}
