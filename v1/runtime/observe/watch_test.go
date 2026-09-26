package observe_test

import (
	"sync"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/observe"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// WATCHED is a run wide enough to report from several goroutines, with a
// retry so one node reaches RUNNING more than once.
const WATCHED = "testdata/watched.star"

// _Watched keeps every change it is told about, and optionally asks for the
// whole run each time.
//
// The lock is the contract rather than an implementation detail: changes
// arrive on the goroutine of the thread that changed, so a run with several
// threads calls this from several at once.
type _Watched struct {
	whole bool

	guard   sync.Mutex
	changes []*workflowpb.Change
	wholes  int
	threads map[string]bool
}

// Changed records the change, and asks for the picture when this watcher is
// the kind that draws one.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func (w *_Watched) Changed(change *workflowpb.Change, whole func() *workflowpb.Workflow) {
	var held *workflowpb.Workflow

	// Asked for outside this watcher's own lock, as any caller would: it is
	// the recorder's lock that matters, and this proves asking from inside
	// Changed does not deadlock against it.
	if w.whole {
		held = whole()
	}

	w.guard.Lock()
	defer w.guard.Unlock()

	w.changes = append(w.changes, change)
	w.threads[change.GetThread()] = true

	if held != nil {
		w.wholes++
	}
}

// _Seen is what this watcher was told, copied out.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func (w *_Watched) _Seen() ([]*workflowpb.Change, int, int) {
	w.guard.Lock()
	defer w.guard.Unlock()

	held := make([]*workflowpb.Change, len(w.changes))
	copy(held, w.changes)

	return held, w.wholes, len(w.threads)
}

// _Watching runs the fixture with a watcher of the given kind.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func _Watching(t *testing.T, whole bool) *_Watched {
	t.Helper()

	into := &_Watched{whole: whole, threads: map[string]bool{}}

	_, err := _Compile(t, WATCHED).Run(
		scheduler.WithReporter(t.Context(), observe.Watching(into)),
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	return into
}

// TestWatch_AskingForTheWholeRunFromInsideChangedDoesNotDeadlock is the hazard
// the notification is shaped around.
//
// The recorder assembles a snapshot under the same lock it records a change
// under, so telling a watcher while holding that lock would deadlock the
// moment the watcher asked for one. It is told after the lock is released.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func TestWatch_AskingForTheWholeRunFromInsideChangedDoesNotDeadlock(t *testing.T) {
	changes, wholes, _ := _Watching(t, true)._Seen()

	if len(changes) == 0 {
		t.Fatal("heard nothing")
	}

	if wholes != len(changes) {
		t.Fatalf("asked for the run %d times over %d changes", wholes, len(changes))
	}
}

// TestWatch_ChangesArriveFromEveryThread checks that a watcher hears about
// spawned lanes and not only the spine.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func TestWatch_ChangesArriveFromEveryThread(t *testing.T) {
	_, _, threads := _Watching(t, false)._Seen()

	// The spine and three spawned lanes.
	if threads != 4 {
		t.Fatalf("heard from %d threads, want 4", threads)
	}
}

// TestWatch_AChangeIsACopy is what stops a watcher seeing a change nobody told
// it about.
//
// A retry drives one node through RUNNING more than once. The recorder keeps
// writing to that node, so a change holding the node itself would have its
// attempt number move underneath whoever kept it.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func TestWatch_AChangeIsACopy(t *testing.T) {
	changes, _, _ := _Watching(t, false)._Seen()

	attempts := map[int32]bool{}

	for _, change := range changes {
		if change.GetNode().GetFunction() != "flaky" {
			continue
		}

		if change.GetNode().GetStatus() != workflowpb.Status_STATUS_RUNNING {
			continue
		}

		attempts[change.GetNode().GetAttempt()] = true
	}

	// Three attempts, each kept as it was when it was reported. Aliasing the
	// node leaves every change showing that node's final state instead, so
	// none of them still reads RUNNING and this finds nothing at all - which
	// is what it does when the copy is removed.
	if len(attempts) != 3 {
		t.Fatalf("kept attempts %v, want one change per attempt", attempts)
	}
}

// TestWatch_ARunWithNoWatcherStillRuns checks that the notification is
// optional, since every run that goes through a store has a recorder and most
// have nothing watching.
//
// Revisions:
//   - 2026-09-23 22:55: initial creation
func TestWatch_ARunWithNoWatcherStillRuns(t *testing.T) {
	built := _Compile(t, WATCHED)

	_, err := built.Run(scheduler.WithReporter(t.Context(), observe.Watching(nil)))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}
