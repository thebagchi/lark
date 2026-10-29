package observe

import (
	"cmp"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	patchpb "github.com/thebagchi/lark/proto/gen/patch"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// NONE is the position of no call, among the calls a thread is in, and the
// ordinal of no call.
const (
	NONE = -1

	// ONCE is a first attempt: a later one advances the call already running
	// rather than making another.
	ONCE = 1

	// FUNCTIONS, CALLS, STATUS, CAUSE and THREADS are the members of a graph's
	// JSON that a change's paths name, as protojson writes them. A test holds
	// them to the schema, so a renamed field cannot leave them behind.
	FUNCTIONS = "functions"
	CALLS     = "calls"
	STATUS    = "status"
	CAUSE     = "cause"
	THREADS   = "threads"

	// ADD and REMOVE are the operations a change is made of, as RFC 6902
	// spells them, and APPEND the index RFC 6901 gives the end of an array.
	ADD    = "add"
	REMOVE = "remove"
	APPEND = "-"

	// SLASH separates the parts of a JSON Pointer.
	SLASH = "/"
)

// _Recorder is the graph of one run, folded from what the run reports, and
// the changes each step makes to it.
//
// One per run, made by Start. It implements the scheduler's Reporter, which is
// how a run tells it anything: nothing here can ask, because nothing here holds
// a run.
//
// callees holds each function the run called, by name, with its node, and
// names holds those names sorted, which is each node's place in the graph;
// edges holds the calls, sorted by caller and then callee. running holds each
// thread in the middle of a call, by id, with the calls it is in. made counts
// the calls started, which is what numbers them. cause is the failure to
// blame, the one whose error Wait returns. status and ending are the run's
// own, as its graph reports them: running, with no cause, until it ends; then
// how it ended and, when it failed, why.
//
// A step looks up the thread and the function it names once, and hands the
// pointers on: a call is a step of every statement that calls, and each lookup
// by a string key is a hash of that string under the lock every thread waits
// for.
//
// arrival counts the times a thread arrived at a node, which numbers each
// arrival. A thread keeps its number, and a function the numbers of the threads
// its node lists, so a thread that leaves is found by a binary search rather
// than by reading the names one by one, which cost the square of the threads in
// one function.
//
// watch is told each step's change, and ops collects it while the step is
// folded; nil when nothing watches, so a run nobody watches builds none. emit
// is held across a step's fold and its telling, so changes arrive in the order
// they were made, and the graph a watcher asks for matches the change it is
// told. watch is set before the run starts and never changes. fault is the
// error a panicking watcher left, written inside emit and read after the run
// ended; the first fault wins, and later steps fold but tell no further change.
//
// blamed is the error of the current cause, kept so _Blame can tell a caller
// that propagated an inner failure from a caller that failed on its own.
type _Recorder struct {
	guard   sync.Mutex
	emit    sync.Mutex
	callees map[string]*_Callee
	names   []string
	edges   []*workflowpb.Edge
	running map[string]*_Thread
	made    int
	cause   *workflowpb.Cause
	blamed  error
	status  workflowpb.Status
	ending  *workflowpb.Cause
	arrival int
	watch   func(change *workflowpb.Change)
	ops     []*patchpb.Operation
	fault   error
}

// _NewRecorder returns a recorder that has been told nothing.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 09:46: prints to standard error by default
//   - 2026-10-02 01:31: holds threads by id rather than nodes by function
//   - 2026-10-02 13:12: prints nothing, a run's printing being its transcript's
//   - 2026-10-02 15:34: holds a node per function and the calls between them
//   - 2026-10-02 16:33: holds the names in order, for the changes it makes
//   - 2026-10-03 21:02: numbers each thread's arrival at a node, so it is found there
//     by a binary search
//   - 2026-10-29 00:25: holds a record per function and per thread, each
//     reached by one lookup
func _NewRecorder() *_Recorder {
	return &_Recorder{
		callees: make(map[string]*_Callee),
		running: make(map[string]*_Thread),
	}
}

// Started records a call a thread began, and tells the watcher what it
// changed.
//
// A line that calls nothing - a join, a sleep, a cancel - folds nothing and
// tells nothing, so it returns before the lock: every thread of a run reports
// through this one recorder, and a step that changes nothing must not wait for
// the others.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-20 20:53: resolves an anonymous function against the graph
//   - 2026-10-02 01:31: takes a Line, which is a statement of its own rather
//     than the function's one node
//   - 2026-10-02 13:12: tells no watcher, there being none
//   - 2026-10-02 15:34: a call of a function's node, with an edge from its
//     caller, rather than a line of a thread
//   - 2026-10-02 16:33: tells the watcher the change it made
//   - 2026-10-28 22:23: takes no lock for a line that calls nothing, and makes
//     the call's record before the lock
//   - 2026-10-29 00:25: makes no record, a call being a value in its thread's
//     calls
func (r *_Recorder) Started(
	thread string,
	line *scheduler.Line,
) {
	if line.Name == "" {
		return
	}

	r._Step(func() {
		r._Start(
			thread,
			line,
		)
	})
}

// Ended records how a call finished, and tells the watcher what it changed.
//
// The error becomes a status here and nowhere else, which is why the scheduler
// hands one over rather than deciding.
//
// A thread cancelled where it stood because a sibling failed reports cancelled,
// not failed. This runtime is fail-fast, so every failing run stops threads
// that were doing nothing wrong, and calling those failures would report one
// broken script as several.
//
// An empty name is a join, a sleep or a cancel, which call nothing: its end
// folds nothing and tells nothing, so it returns before the lock that every
// thread of the run shares.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-20 11:34: tells a cancellation from a failure
//   - 2026-09-20 11:36: carries the failure's text, for a reader that needs
//     more than a colour
//   - 2026-09-23 22:48: tells the watcher, outside the lock
//   - 2026-10-02 01:31: closes the line that called name rather than the
//     function's one node
//   - 2026-10-02 13:12: tells no watcher, there being none
//   - 2026-10-02 15:34: closes a call, whose function's node shows it only
//     while it is that function's newest
//   - 2026-10-02 16:33: tells the watcher the change it made
//   - 2026-10-28 22:23: takes no lock for an end that calls nothing
func (r *_Recorder) Ended(
	thread string,
	name string,
	err error,
) {
	if name == "" {
		return
	}

	r._Step(func() {
		r._End(
			thread,
			name,
			err,
		)
	})
}

// _Start folds a call a thread began into the graph.
//
// The line calls a function; Started filters a line that calls nothing. The
// call is numbered here, since the number is the run's count of calls.
//
// A later attempt of a repeat or a retry is the call already running, not
// another, so repeat(3, step) is one call. Anything else is a new call: its
// function's node runs, shows this call from now on, and gains an edge from
// the call the thread is in the middle of, when it is in one; and the thread
// moves to the node it is now executing.
//
// A spawn's call runs on the thread it started and ends there, so it is that
// thread's first call - and its caller is whatever the spawning thread is in
// the middle of. That thread is the lane here, and the one that reported the
// line is the current thread; for any other call the two are one.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 01:31: initial creation
//   - 2026-10-02 13:12: returns no change, nothing being told one
//   - 2026-10-02 15:34: a call of a function's node, with an edge from the
//     thread's innermost call
//   - 2026-10-02 16:33: moves the thread to the node it now executes
//   - 2026-10-28 22:23: numbers the record Started made, rather than making
//     one under the lock
//   - 2026-10-29 00:25: looks up each thread and the function once, and keeps
//     the call as a value in the lane's calls
func (r *_Recorder) _Start(
	thread string,
	line *scheduler.Line,
) {
	current := r.running[thread]

	if current != nil && line.Attempt > ONCE {
		advancing := current._Newest(r.callees[line.Name]) != NONE
		if advancing {
			return
		}
	}

	var caller *_Callee

	if current != nil {
		caller = current._Innermost()
	}

	var (
		id   = thread
		lane = current
	)

	if line.Child != "" {
		id = line.Child
		lane = r.running[id]
	}

	if lane == nil {
		lane = &_Thread{
			name: id,
		}
		r.running[id] = lane
	}

	var (
		executing = lane._Innermost()
		callee    = r._Running(line.Name)
	)

	r.made++
	callee.newest = r.made
	lane.calls = append(
		lane.calls,
		_Call{
			callee:  callee,
			ordinal: r.made,
		},
	)

	r._Moved(
		lane,
		executing,
		callee,
	)

	if caller != nil {
		r._Edge(
			caller.node.GetName(),
			line.Name,
		)
	}
}

// _End folds a call's end into the graph.
//
// It closes the newest call to name's function on that thread: calls on one
// thread start and end in order, so the newest is the one that ended. The
// thread moves back to whatever it is executing now, and the function's node
// takes the status only while this is its newest call, since a node shows its
// newest call. name is never empty, Ended filtering an end that calls
// nothing; an end that matches no call is one this recorder was never told
// began, and changes nothing.
//
// A thread left in no call is forgotten, so a long run holds only the threads
// still going.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 01:31: initial creation
//   - 2026-10-02 13:12: returns no change, nothing being told one
//   - 2026-10-02 15:34: closes the thread's newest call to the function, and
//     sets its node only while it is the function's newest
//   - 2026-10-02 16:33: moves the thread back to what it executes now
//   - 2026-10-28 22:23: leaves an empty name to Ended, which filters it
//   - 2026-10-29 00:25: looks up the thread and the function once, and forgets
//     a thread left in no call
func (r *_Recorder) _End(
	thread string,
	name string,
	err error,
) {
	var (
		lane   = r.running[thread]
		callee = r.callees[name]
	)

	if lane == nil || callee == nil {
		return
	}

	var (
		executing = lane._Innermost()
		ordinal   = lane._Close(callee)
	)

	if ordinal == NONE {
		return
	}

	r._Moved(
		lane,
		executing,
		lane._Innermost(),
	)

	if len(lane.calls) == 0 {
		delete(
			r.running,
			thread,
		)
	}

	status := _Became(err)

	if callee.newest == ordinal {
		r._Status(
			callee,
			status,
		)
	}

	r._Blame(
		name,
		status,
		err,
	)
}

// _Began records that the run is going, which is the first change a watcher
// is told: the empty graph a host starts from becomes a running one.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-02 16:45: keeps the status for the graph, watched or not
func (r *_Recorder) _Began() {
	r._Step(func() {
		r.status = workflowpb.Status_STATUS_RUNNING

		if r.watch == nil {
			return
		}

		r._Op(
			ADD,
			_Pointer(STATUS),
			_Status(r.status),
		)
	})
}

// _Finished records how the run ended, and the cause when it failed, which is
// the last change a watcher is told.
//
// Kept here rather than read from the run, so the graph turns finished in the
// same step as the change saying so: a watcher asking from inside that change
// is told the run ended, though the run has yet to close Done.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-02 16:45: keeps the status and the cause for the graph, watched
//     or not
func (r *_Recorder) _Finished(
	status workflowpb.Status,
	cause *workflowpb.Cause,
) {
	r._Step(func() {
		r.status = status
		r.ending = cause

		if r.watch == nil {
			return
		}

		r._Op(
			ADD,
			_Pointer(STATUS),
			_Status(status),
		)

		if cause != nil {
			r._Op(
				ADD,
				_Pointer(CAUSE),
				_Valued(cause),
			)
		}
	})
}

// _Step folds one step of the run under the lock, then tells the watcher the
// change it made.
//
// With a watcher, the telling lock is held across the fold and the telling, so
// no other step folds while a change is told: changes reach the watcher in the
// order they were made, and a watcher asking for the whole graph from inside
// gets exactly the graph after the change it is being told. The watcher runs
// without the fold's lock, which asking takes. Without one, a step is the fold
// alone.
//
// A panic in the watcher is contained and kept as the recorder's fault, so a
// broken callback does not crash the process. The first fault wins; later
// steps still fold, but no further change is told.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-24 16:10: contain a panic in the change callback, so it fails
//     the run instead of the process
func (r *_Recorder) _Step(fold func()) {
	if r.watch == nil {
		r.guard.Lock()
		defer r.guard.Unlock()

		fold()

		return
	}

	r.emit.Lock()
	defer r.emit.Unlock()

	r.guard.Lock()

	fold()

	ops := r.ops
	r.ops = nil

	r.guard.Unlock()

	if len(ops) == 0 || r.fault != nil {
		return
	}

	err := guard.Contained(func() {
		r.watch(&workflowpb.Change{
			Operations: ops,
		})
	})
	if err != nil {
		r.fault = err
	}
}

// _Op adds one operation to the change this step is making.
//
// Every caller asks whether anything watches before it builds a path or a
// value, so a run nobody watches builds neither: it pays for its graph and
// nothing more.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func (r *_Recorder) _Op(
	op string,
	path string,
	value *structpb.Value,
) {
	r.ops = append(
		r.ops,
		&patchpb.Operation{
			Op:    op,
			Path:  path,
			Value: value,
		},
	)
}

// _Running makes the node of the function name running, adding it in its
// place when the run first calls it, and returns the function's record.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation, from _Node
//   - 2026-10-29 00:25: returns the function's record, so its caller needs no
//     second lookup
func (r *_Recorder) _Running(name string) *_Callee {
	callee, known := r.callees[name]
	if known {
		r._Status(
			callee,
			workflowpb.Status_STATUS_RUNNING,
		)

		return callee
	}

	node := &workflowpb.Node{
		Name:   name,
		Status: workflowpb.Status_STATUS_RUNNING,
	}
	callee = &_Callee{
		node: node,
	}
	r.callees[name] = callee

	idx, _ := slices.BinarySearch(
		r.names,
		name,
	)
	r.names = slices.Insert(
		r.names,
		idx,
		name,
	)

	if r.watch == nil {
		return callee
	}

	if len(r.names) == 1 {
		r._Op(
			ADD,
			_Pointer(FUNCTIONS),
			_Listed(_Valued(node)),
		)

		return callee
	}

	r._Op(
		ADD,
		_Pointer(
			FUNCTIONS,
			idx,
		),
		_Valued(node),
	)

	return callee
}

// _Status sets the status of callee's node.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-29 00:25: takes the function's record rather than its name
func (r *_Recorder) _Status(
	callee *_Callee,
	status workflowpb.Status,
) {
	callee.node.Status = status

	if r.watch == nil {
		return
	}

	r._Op(
		ADD,
		_Pointer(
			FUNCTIONS,
			r._Place(callee.node.GetName()),
			STATUS,
		),
		_Status(status),
	)
}

// _Moved moves thread from the node of the function it was executing to the
// node of the one it executes now. Either may be nil: a thread starting
// executes nothing before, and one whose last call ended nothing after.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-29 00:25: takes the thread's record and the functions' records,
//     which its caller already holds
func (r *_Recorder) _Moved(
	thread *_Thread,
	from *_Callee,
	to *_Callee,
) {
	if from == to {
		return
	}

	if from != nil {
		r._Left(
			thread,
			from,
		)
	}

	if to != nil {
		r._Joined(
			thread,
			to,
		)
	}
}

// _Left takes thread off the threads executing callee.
//
// A node left with none loses the member, as protojson writes an empty list:
// not at all.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 21:02: finds the thread by its arrival number, in a binary search,
//     rather than by reading every name the node lists
//   - 2026-10-29 00:25: searches the function's own arrival numbers, with no
//     lookup of each thread it passes
func (r *_Recorder) _Left(
	thread *_Thread,
	callee *_Callee,
) {
	idx, found := slices.BinarySearch(
		callee.tickets,
		thread.arrived,
	)
	if !found {
		return
	}

	callee.node.Threads = slices.Delete(
		callee.node.Threads,
		idx,
		idx+1,
	)
	callee.tickets = slices.Delete(
		callee.tickets,
		idx,
		idx+1,
	)

	r._Removed(
		callee,
		idx,
	)
}

// _Removed reports that the thread at idx left callee's node, when a watch is
// attached.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 00:25: takes the function's record rather than its name
func (r *_Recorder) _Removed(
	callee *_Callee,
	idx int,
) {
	if r.watch == nil {
		return
	}

	var (
		node  = callee.node
		place = r._Place(node.GetName())
	)

	if len(node.Threads) == 0 {
		r._Op(
			REMOVE,
			_Pointer(
				FUNCTIONS,
				place,
				THREADS,
			),
			nil,
		)

		return
	}

	r._Op(
		REMOVE,
		_Pointer(
			FUNCTIONS,
			place,
			THREADS,
			idx,
		),
		nil,
	)
}

// _Joined adds thread to the threads executing callee.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 21:02: numbers the arrival, so the thread is found again by it
//   - 2026-10-29 00:25: keeps the number on the thread and beside the node's
//     list, rather than in a map by thread
func (r *_Recorder) _Joined(
	thread *_Thread,
	callee *_Callee,
) {
	node := callee.node
	node.Threads = append(
		node.Threads,
		thread.name,
	)

	r.arrival++
	thread.arrived = r.arrival
	callee.tickets = append(
		callee.tickets,
		r.arrival,
	)

	if r.watch == nil {
		return
	}

	var (
		place = r._Place(node.GetName())
		lane  = structpb.NewStringValue(thread.name)
	)

	if len(node.Threads) == 1 {
		r._Op(
			ADD,
			_Pointer(
				FUNCTIONS,
				place,
				THREADS,
			),
			_Listed(lane),
		)

		return
	}

	r._Op(
		ADD,
		_Pointer(
			FUNCTIONS,
			place,
			THREADS,
			APPEND,
		),
		lane,
	)
}

// _Edge records that caller called callee, once however often it does.
//
// The search compares names, and only an edge the graph lacks is allocated:
// most calls repeat an edge, and this runs under the lock every thread of the
// run shares.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-10-02 16:33: keeps the edges in order, adding each in its place
//   - 2026-10-28 22:23: allocates an edge only when the graph lacks it
func (r *_Recorder) _Edge(
	caller string,
	callee string,
) {
	idx, found := slices.BinarySearchFunc(
		r.edges,
		callee,
		func(
			edge *workflowpb.Edge,
			target string,
		) int {
			return _Order(
				edge,
				caller,
				target,
			)
		},
	)
	if found {
		return
	}

	edge := &workflowpb.Edge{
		Caller: caller,
		Callee: callee,
	}
	r.edges = slices.Insert(
		r.edges,
		idx,
		edge,
	)

	if r.watch == nil {
		return
	}

	if len(r.edges) == 1 {
		r._Op(
			ADD,
			_Pointer(CALLS),
			_Listed(_Valued(edge)),
		)

		return
	}

	r._Op(
		ADD,
		_Pointer(
			CALLS,
			idx,
		),
		_Valued(edge),
	)
}

// _Place is the position of the function name's node in the graph.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func (r *_Recorder) _Place(name string) int {
	idx, _ := slices.BinarySearch(
		r.names,
		name,
	)

	return idx
}

// _Blame remembers which call to blame for a failure, so Cause names the
// function whose error Wait returns.
//
// It overwrites an earlier blame when a later failure arrives, unless the
// two errors share a chain. The check is bidirectional: the new error may
// wrap the old, or the old may wrap the new, because Starlark and the
// scheduler can wrap the same cause in different directions. A shared chain
// means one error propagated the other, not a new failure.
//
// join picks the first failure in argument order, and it waits for each
// handle in turn. A function that failed before the chosen one in wall
// time but after it in argument order is overwritten when the chosen one
// arrives. A caller that propagated the chosen one is not.
//
// A thread stopped because something else failed reports a cancellation,
// not a failure, so only a real failure lands here.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
//   - 2026-09-21 01:21: takes the node's identity, which the key holds, so the
//     message needs no thread of its own
//   - 2026-09-21 08:09: reached through a pointer, as every struct here is
//   - 2026-10-02 01:31: takes the thread and the line's position, which the
//     cause carries as its index
//   - 2026-10-02 15:34: takes the function, its status and its error, a cause
//     naming the function alone
//   - 2026-10-24 20:30: overwrite unless the errors share a chain, so Cause
//     names the function whose error Wait returns
func (r *_Recorder) _Blame(
	name string,
	status workflowpb.Status,
	err error,
) {
	if status != workflowpb.Status_STATUS_FAILED {
		return
	}

	propagated := r.blamed != nil && (errors.Is(
		err,
		r.blamed,
	) || errors.Is(
		r.blamed,
		err,
	))
	if propagated {
		return
	}

	r.cause = &workflowpb.Cause{
		Function: name,
		Failure:  _Why(err),
	}
	r.blamed = err
}

// _Because is the cause to report for a run that ended this way, or nil.
//
// Nil unless the run failed, so a host tests the pointer rather than the enum.
//
// A run can fail with no call blamed - a failure the scheduler saw and no
// thread did. Then the run's own text stands in, with no function named:
// something went wrong and this is all that is known, which is more use than a
// nil a host reads as "nothing failed".
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
func (r *_Recorder) _Because(
	status workflowpb.Status,
	failure string,
) *workflowpb.Cause {
	if status != workflowpb.Status_STATUS_FAILED {
		return nil
	}

	found := r._Cause()
	if found != nil {
		return found
	}

	return &workflowpb.Cause{
		Failure: failure,
	}
}

// _Cause is the call whose failure ended the run, or nil.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
func (r *_Recorder) _Cause() *workflowpb.Cause {
	r.guard.Lock()
	defer r.guard.Unlock()

	return r.cause
}

// _Fault is the error a panicking watcher left, or nil.
//
// Not under the guard: fault is written inside emit, and read after the run
// ended, so the happens-before edge is the goroutine join (Wait, or the
// caller's own check in Start).
//
// Revisions:
//   - 2026-10-24 16:10: initial creation
func (r *_Recorder) _Fault() error {
	return r.fault
}

// _Drawn is the run's graph as it stands: the run's status, every function's
// node, sorted by name, every call, sorted by caller and then callee, and what
// ended the run when it failed.
//
// Cloned, not aliased. The recorder keeps writing to its nodes after a graph is
// handed out, so sharing one would let a finished report change under whoever
// is reading it. A clone rather than a copy of the fields, because a field
// added to Node later would be dropped by a copy and nothing would say so.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation, replacing _Threads
//   - 2026-10-02 16:33: reads the names and the edges in the order they are
//     kept
//   - 2026-10-02 16:45: carries the run's status and cause, which the run now
//     records here
func (r *_Recorder) _Drawn() *workflowpb.Graph {
	r.guard.Lock()
	defer r.guard.Unlock()

	graph := &workflowpb.Graph{
		Status: r.status,
	}

	if r.ending != nil {
		graph.Cause = proto.CloneOf(r.ending)
	}

	for _, name := range r.names {
		graph.Functions = append(
			graph.Functions,
			proto.CloneOf(r.callees[name].node),
		)
	}

	for _, edge := range r.edges {
		graph.Calls = append(
			graph.Calls,
			proto.CloneOf(edge),
		)
	}

	return graph
}

// _Order orders edge against the edge from caller to callee, as a graph lists
// them: by caller, then callee.
//
// The other edge comes as its two names rather than as an edge, so a search
// for one the graph may already hold allocates nothing.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-28 22:23: takes the other edge as its caller and callee
func _Order(
	edge *workflowpb.Edge,
	caller string,
	callee string,
) int {
	return cmp.Or(
		strings.Compare(
			edge.GetCaller(),
			caller,
		),
		strings.Compare(
			edge.GetCallee(),
			callee,
		),
	)
}

// _Pointer is the JSON Pointer that parts name, each a member or an index.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Pointer(parts ...any) string {
	var held strings.Builder

	for _, part := range parts {
		held.WriteString(SLASH)

		switch named := part.(type) {
		case int:
			held.WriteString(strconv.Itoa(named))
		case string:
			held.WriteString(named)
		}
	}

	return held.String()
}

// _Status is status as JSON writes it: by the enum's name.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Status(status workflowpb.Status) *structpb.Value {
	return structpb.NewStringValue(status.String())
}

// _Listed is value as the one element of a list.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Listed(value *structpb.Value) *structpb.Value {
	return structpb.NewListValue(&structpb.ListValue{
		Values: []*structpb.Value{value},
	})
}

// _Valued is message as JSON writes it, so a change carries exactly what the
// graph's own JSON holds.
//
// Through protojson rather than a field-by-field copy, which would be a second
// declaration of the schema's shape. protojson refuses only text that is not
// valid UTF-8, and a name, a thread id and a failure here come from Starlark
// strings and Go errors; should one ever not be, a null stands in, so the
// broken invariant shows in a host's copy rather than taking the run down.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Valued(message proto.Message) *structpb.Value {
	raw, err := protojson.Marshal(message)
	if err != nil {
		return structpb.NewNullValue()
	}

	value := new(structpb.Value)

	err = value.UnmarshalJSON(raw)
	if err != nil {
		return structpb.NewNullValue()
	}

	return value
}
