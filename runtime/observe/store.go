// Package observe starts runs that outlive the call starting them, and holds
// them by an id until somebody asks how they ended.
//
// A host with a user interface starts a script and is given an id back at once,
// then asks about that id for as long as it likes. Nothing here blocks the
// caller and nothing here keeps a run alive: a run is a goroutine evaluating an
// artifact, and this package is the map that finds it again.
package observe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/artifact"
	"github.com/thebagchi/lark/runtime/guard"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// Option is something a run is started with.
//
// An option rather than a parameter, so a call that was written before any of
// these existed still compiles.
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

// TTL is how long a finished run is kept for somebody to read its ending.
//
// A day rather than minutes, because the interface this serves is one somebody
// opens the next morning to see what ran overnight. A sweep is not a race a
// user should lose.
const TTL = 24 * time.Hour

var (
	// ErrUnknown is returned for an id no run answers to. An id that never
	// existed and an id already collected give the same answer, because the
	// store cannot tell them apart once it has forgotten one.
	ErrUnknown = errors.New("no such run")
)

// Store is every run started through it, live and finished.
//
// A store is safe for any number of goroutines. Starting a run, finding one and
// counting them all happen under one lock, because an id is handed out and
// recorded in the same moment and a caller that could see one without the other
// would see a run that does not exist yet.
type Store struct {
	guard   sync.RWMutex
	entries map[string]*_Entry
}

// New returns a store holding no runs.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func New() *Store {
	return &Store{
		entries: make(map[string]*_Entry),
	}
}

// Start evaluates art's entry point on a goroutine of its own and returns at
// once with the id that finds it again.
//
// The context passed in is the parent of the run's, so a caller keeps the
// ability to stop everything without this package needing to hold a context.
//
// The id is a version 7 UUID. Not a counter: a counter re-issues its first id
// after a restart, so an id bookmarked by an interface would silently attach to
// a different run and report one run's state under another's name. A UUID
// cannot be re-issued, so a stale id is always unknown, which is the honest
// answer. Version 7 rather than 4 because a v7 id opens with the millisecond it
// was made, so a list of ids is already in the order the runs started.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-20 01:41: takes options, builds the run a recorder, and puts that
//     recorder where the scheduler will find it
//   - 2026-09-20 11:56: guards the goroutine, so a panic before the script is
//     reached fails the run instead of the process
func (s *Store) Start(
	ctx context.Context,
	art *artifact.Artifact,
	opts ...Option,
) string {
	into := _NewRecorder()

	for _, opt := range opts {
		opt(into)
	}

	inner, stop := context.WithCancel(scheduler.WithReporter(ctx, into))

	entry := &_Entry{
		id:   uuid.Must(uuid.NewV7()).String(),
		stop: stop,
		done: make(chan struct{}),
		into: into,
	}

	s.guard.Lock()
	s.entries[entry.id] = entry
	s.guard.Unlock()

	s._Sweep(time.Now())

	go func() {
		defer close(entry.done)
		defer stop()

		// Guarded, because this is a goroutine of ours and a panic on it cannot
		// be recovered from outside - it would take the host down rather than
		// fail the run. Invoke guards the script's own evaluation; this guards
		// everything before it gets there, which is where an artifact that is
		// not one lands.
		guard.WithRecover(
			&entry.value,
			&entry.err,
			func() (starlark.Value, error) {
				return art.Run(inner)
			},
		)

		entry.ended = time.Now()
	}()

	return entry.id
}

// Status returns how the run with this id is doing.
//
// The run's status is the run's own, never folded from its functions': a run
// that succeeded without reaching everything its graph declared is succeeded,
// and the functions it did not reach stay pending. Folding would make such a
// run report itself pending for ever, since a pending node has nothing further
// to happen to it.
//
// Reading does not forget. Every caller that asks is told how the run ended,
// for as long as the store holds it, because two interfaces watching one run is
// ordinary and the first to ask should not be the only one answered.
//
// The sweep is the only thing that forgets, TTL after a run ended, read or not.
// So a finished run holds what it produced for a day whichever way it went -
// that is the exchange, memory for delivery, and it is why the ttl exists.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-20 12:04: stops forgetting a run it reported, so two interfaces
//     watching one run are both answered
func (s *Store) Status(id string) (*workflowpb.Workflow, error) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s._Reap(time.Now())

	entry, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("%s: %w", id, ErrUnknown)
	}

	if !entry._Over() {
		return &workflowpb.Workflow{
			Status:  workflowpb.Status_STATUS_RUNNING,
			Threads: entry.into._Threads(),
		}, nil
	}

	status, failure := entry._Ending()

	return &workflowpb.Workflow{
		Status:  status,
		Threads: entry.into._Threads(),
		Cause:   entry.into._Because(status, failure),
	}, nil
}

// _Sweep forgets every finished run that ended longer ago than a caller had to
// read it.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (s *Store) _Sweep(now time.Time) {
	s.guard.Lock()
	defer s.guard.Unlock()

	s._Reap(now)
}

// _Reap is _Sweep without the lock, for callers already holding it.
//
// The guard on completion comes before any clock, and that ordering is not a
// style choice. A run still going has never had ended written, so it holds the
// zero time - the year 1 - and any elapsed-time test would find it older than
// any age and evict it while it was still working. The run would carry on and
// its id would go unknown, which reads as a lost id rather than as a sweep.
//
// The same ordering is what makes the read safe: _Over reads done, and done is
// the edge that publishes ended.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (s *Store) _Reap(now time.Time) {
	for id, entry := range s.entries {
		if !entry._Over() {
			continue
		}

		if now.Sub(entry.ended) > TTL {
			delete(s.entries, id)
		}
	}
}

// Size is how many runs this store still holds.
//
// It exists for a host that wants to watch what the store is keeping, and for
// the tests that measure it. A store only grows when the host calls Start.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (s *Store) Size() int {
	s.guard.RLock()
	defer s.guard.RUnlock()

	return len(s.entries)
}

// _Find returns the entry an id names.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (s *Store) _Find(id string) (*_Entry, error) {
	s.guard.RLock()
	defer s.guard.RUnlock()

	entry, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("%s: %w", id, ErrUnknown)
	}

	return entry, nil
}

// Wait blocks until the run with this id is over and returns what it produced.
//
// The context is the caller's patience, not the run's. A run that never ends
// would otherwise trap whoever waited on it, and their only escape would be
// stopping the run - a different intention. Giving up here leaves the run
// untouched.
//
// Waiting does not forget the run, and neither does anything else a caller
// does. The sweep is the only thing that forgets.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
//   - 2026-09-20 12:04: the asymmetry this comment explained is gone, Status
//     having stopped forgetting too
func (s *Store) Wait(ctx context.Context, id string) (starlark.Value, error) {
	entry, err := s._Find(id)
	if err != nil {
		return nil, err
	}

	select {
	case <-entry.done:
		return entry.value, entry.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w", id, ctx.Err())
	}
}

// Cancel stops the run with this id, without waiting for it.
//
// A caller wanting both calls Cancel and then Wait, which reads as what it is.
//
// Cancelling a finished run is not an error. The run is over, the cancellation
// reaches nothing, and a caller racing a run to its end should not have to care
// which of them won. Only an id nothing answers to is refused.
//
// Revisions:
//   - 2026-09-20 01:36: initial creation
func (s *Store) Cancel(id string) error {
	entry, err := s._Find(id)
	if err != nil {
		return err
	}

	entry.stop()

	return nil
}
