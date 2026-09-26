// Package observe starts runs that outlive the call starting them, and reports
// what they are doing while they do it.
//
// A host starts a script and is handed the run itself, which it stops, waits
// for and asks about for as long as it holds it. Nothing here blocks the
// caller and nothing here keeps a run alive - a run is a goroutine evaluating
// an artifact.
//
// Nothing here keeps a finished run either. What is worth remembering about
// one, and for how long, is the host's decision and not this runtime's, so
// there is no store, no identifier to look a run up by, and nothing that
// forgets on a schedule of its own. A host that wants a run after it has let
// go of it keeps what it was told, as it is told it: WithWatcher reports every
// change as it happens.
package observe

import (
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Option is something a run is started with.
//
// An option rather than a parameter, so a call that was written before any of
// these existed still compiles. It reaches an unexported recorder, so only
// this package can write one: what a run is started with is this package's
// business, and a host chooses among the options here rather than adding its
// own.
type Option func(into *_Recorder)

// WithGraph tells a run what its script could do, so a report can say what has
// not happened yet.
//
// A graph is a floor, never a ceiling. It may add functions and threads that
// have not happened, and it may never remove, rename or hide one that did: a
// run that spawns a function no graph mentions is reported anyway, beside the
// pending ones. A graph is a host's claim about a script; the run is what
// happened; when they disagree both are visible.
//
// A nil graph is the same as no graph. An option that refuses is an option a
// caller has to check, and this one has nothing to fail at.
//
// Revisions:
//   - 2026-09-20 01:40: initial creation
func WithGraph(graph *workflowpb.Graph) Option {
	return func(into *_Recorder) {
		if graph == nil {
			return
		}

		into._Seed(graph)
	}
}

// Pending is a graph as a workflow that has not started: every thread it
// declares, every function it names, all waiting.
//
// What a user interface draws before anything runs, from the graph a bundle
// carries. It is the same seeding a run does for the functions it has not
// reached yet, which is why it lives here and not beside the graph: a status
// is this package's word, and a graph carries none.
//
// A nil graph is an empty workflow rather than a refusal, as it is for
// WithGraph: a bundle whose script a graph could not carry has none, and a
// reader of one should get an empty drawing rather than an error.
//
// Revisions:
//   - 2026-09-21 17:19: initial creation
func Pending(graph *workflowpb.Graph) *workflowpb.Workflow {
	into := _NewRecorder()

	into._Seed(graph)

	return &workflowpb.Workflow{
		Status:  workflowpb.Status_STATUS_PENDING,
		Threads: into._Threads(),
	}
}

// WithLog is the file a run's transcript is written to, each line behind the
// thread that printed it.
//
// A path rather than a directory. The directory named one and the file was
// called after the identifier this package used to mint - so with nothing
// minting a name, naming the run is the caller's, which is where it belonged:
// a host knows what this run is and a runtime does not.
//
// The directory is made if it is not there. A run whose file cannot be opened
// fails, before its script is evaluated, rather than running with its output
// going nowhere a caller asked for.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation, as WithLogs, taking the directory a
//     run's file was named inside
//   - 2026-09-23 23:20: takes the file, there being no identifier left to name
//     one after
func WithLog(path string) Option {
	return func(into *_Recorder) {
		into.file = path
	}
}

// WithPrinter tells a run where what its script prints goes.
//
// Without it a line goes to standard error, which is where the interpreter's
// default would have put it.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func WithPrinter(print func(string)) Option {
	return func(into *_Recorder) {
		into.print = print
	}
}
