package observe

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	patchpb "github.com/thebagchi/lark/proto/gen/patch"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
)

// NONE is the position of no call, among the calls a thread is in, and the
// ordinal of no call.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	NONE = -1

	// ONCE is a first attempt: a later one advances the call already running
	// rather than making another.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ONCE = 1

	// FUNCTIONS, CALLS, STATUS, CAUSE and THREADS are the members of a graph's
	// JSON that a change's paths name, as protojson writes them. A test holds
	// them to the schema, so a renamed field cannot leave them behind.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	FUNCTIONS = "functions"
	CALLS     = "calls"
	STATUS    = "status"
	CAUSE     = "cause"
	THREADS   = "threads"

	// ADD and REMOVE are the operations a change is made of, as RFC 6902
	// spells them, and APPEND the index RFC 6901 gives the end of an array.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ADD    = "add"
	REMOVE = "remove"
	APPEND = "-"

	// SLASH separates the parts of a JSON Pointer.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	SLASH = "/"

	// DECIMAL is the base an index is written in.
	//
	// Revisions:
	//   - 2026-11-02 16:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	DECIMAL = 10

	// STEP_CAPACITY is the slice capacity one step reserves for its operations,
	// which is the most one step tells. A start tells four: the
	// status of the function (or its node), the thread leaving a node, the
	// thread joining one, and the edge from the caller. An end tells three.
	//
	// Revisions:
	//   - 2026-11-02 17:06: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STEP_CAPACITY = 4
)

// Recorder is the graph of one run, folded from what the run reports, and
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
// the calls started, which is what numbers them. The blamer holds the failure
// to blame, the one whose error Wait returns. status and ending are the run's
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
// _Blamer is the failure to blame, kept by the rule the ledger keeps it by
// too; its methods are the recorder's own.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: renamed from _Recorder and the fields ops, running, watch, as a test in
//     tests/ needs them (C120a, Q314)
type Recorder struct {
	guard   sync.Mutex
	emit    sync.Mutex
	callees map[string]*_Callee
	names   []string
	edges   []*workflowpb.Edge
	Running map[string]*_Thread
	made    int
	_Blamer
	status  workflowpb.Status
	ending  *workflowpb.Cause
	arrival int
	Watch   func(change *workflowpb.Change)
	Ops     []*patchpb.Operation
	fault   error
}

// NewRecorder returns a recorder that has been told nothing.
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
//   - 2026-11-10 00:25: renamed from _NewRecorder, as a test in tests/ needs it (C120a, Q314)
func NewRecorder() *Recorder {
	return &Recorder{
		callees: make(map[string]*_Callee),
		Running: make(map[string]*_Thread),
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
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) Started(
	thread string,
	line *common.Line,
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
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) Ended(
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

// Began records that the run is going, which is the first change a watcher
// is told: the empty graph a host starts from becomes a running one.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-02 16:45: keeps the status for the graph, watched or not
//   - 2026-11-02 08:25: calls _Statused, renamed from _Status
//   - 2026-11-10 00:25: renamed from _Began, as a test in tests/ needs it (C120a, Q314)
func (r *Recorder) Began() {
	r._Step(func() {
		r.status = workflowpb.Status_STATUS_RUNNING

		if r.Watch == nil {
			return
		}

		r.Op(
			ADD,
			Pointer(STATUS),
			_Statused(r.status),
		)
	})
}

// Unstarted adds a node NOT_STARTED, and an edge from its caller, for each
// spawn the script writes in a function the run called, of a function it did
// not, and tells the watcher the change.
//
// Which sites count is _Prevented's, read against the graph as it stands before
// any is added, so a node added here brings no site of its own in. It is one
// step, so a watcher is told it as one change, before the change that ends the
// run.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-10 00:25: renamed from _Unstarted, as a test in tests/ needs it (C120a, Q314)
func (r *Recorder) Unstarted(sites []common.Spawn) {
	if len(sites) == 0 {
		return
	}

	r._Step(func() {
		prevented := _Prevented(
			sites,
			func(name string) bool {
				return r.callees[name] != nil
			},
		)

		for _, site := range prevented {
			if r.callees[site.Target] == nil {
				r._Added(
					site.Target,
					workflowpb.Status_STATUS_NOT_STARTED,
				)
			}

			r._Edge(
				site.Caller,
				site.Target,
			)
		}
	})
}

// Finished records how the run ended, and the cause when it failed, which is
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
//   - 2026-11-02 08:25: calls _Statused, renamed from _Status
//   - 2026-11-10 00:25: renamed from _Finished, as a test in tests/ needs it (C120a, Q314)
func (r *Recorder) Finished(
	status workflowpb.Status,
	cause *workflowpb.Cause,
) {
	r._Step(func() {
		r.status = status
		r.ending = cause

		if r.Watch == nil {
			return
		}

		r.Op(
			ADD,
			Pointer(STATUS),
			_Statused(status),
		)

		if cause != nil {
			r.Op(
				ADD,
				Pointer(CAUSE),
				_Valued(cause),
			)
		}
	})
}

// Op adds one operation to the change this step is making.
//
// Every caller asks whether anything watches before it builds a path or a
// value, so a run nobody watches builds neither: it pays for its graph and
// nothing more.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-02 16:52: makes room for a step's most operations when the step
//     adds its first, so the slice does not grow 1, 2, 4
//   - 2026-11-02 17:01: reserves STEP_CAPACITY, renamed from STEPOPS
//   - 2026-11-10 00:25: renamed from _Op, as a test in tests/ needs it (C120a, Q314)
func (r *Recorder) Op(
	op string,
	path string,
	value *structpb.Value,
) {
	if r.Ops == nil {
		r.Ops = make(
			[]*patchpb.Operation,
			0,
			STEP_CAPACITY,
		)
	}

	r.Ops = append(
		r.Ops,
		&patchpb.Operation{
			Op:    op,
			Path:  path,
			Value: value,
		},
	)
}

// Drawn is the run's graph as it stands: the run's status, every function's
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
//   - 2026-11-02 22:33: is _Standing, the same graph (C27)
//   - 2026-11-10 00:25: renamed from _Drawn, as a test in tests/ needs it (C120a, Q314)
func (r *Recorder) Drawn() *workflowpb.Graph {
	return r._Standing()
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
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 01:00: now uses Recorder, Running, Calls, were _Recorder, running, calls
//     (C120a, Q314)
func (r *Recorder) _Start(
	thread string,
	line *common.Line,
) {
	current := r.Running[thread]

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
		lane = r.Running[id]
	}

	if lane == nil {
		lane = &_Thread{
			name: id,
		}
		r.Running[id] = lane
	}

	var (
		executing = lane._Innermost()
		callee    = r._Running(line.Name)
	)

	r.made++
	callee.newest = r.made
	lane.Calls = append(
		lane.Calls,
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
//   - 2026-11-10 01:00: now uses Recorder, Running, Calls, Became, were _Recorder, running, calls,
//     _Became (C120a, Q314)
func (r *Recorder) _End(
	thread string,
	name string,
	err error,
) {
	var (
		lane   = r.Running[thread]
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

	if len(lane.Calls) == 0 {
		delete(
			r.Running,
			thread,
		)
	}

	status := Became(err)

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
//   - 2026-11-10 01:00: now uses Recorder, Watch, Ops, were _Recorder, watch, ops (C120a, Q314)
func (r *Recorder) _Step(fold func()) {
	if r.Watch == nil {
		r.guard.Lock()
		defer r.guard.Unlock()

		fold()

		return
	}

	r.emit.Lock()
	defer r.emit.Unlock()

	r.guard.Lock()

	fold()

	ops := r.Ops
	r.Ops = nil

	r.guard.Unlock()

	if len(ops) == 0 || r.fault != nil {
		return
	}

	err := guard.Contained(func() {
		r.Watch(&workflowpb.Change{
			Operations: ops,
		})
	})
	if err != nil {
		r.fault = err
	}
}

// _Enlist reports that value joined an array of the graph. size is how many
// elements the array holds now, value included.
//
// parts is the path of the element: the array's path, then the element's index
// or APPEND. protojson leaves out an array with no elements, so the first
// element has no array to be added to (RFC 6902, section 4.1: an array
// holding the target must exist). The change then adds the whole array
// [value] at the array's own path, which is parts without its last part.
//
// Callers hold the lock and have seen that a watch is attached.
//
// Revisions:
//   - 2026-11-02 00:54: initial creation, from the three places that added to
//     a list
//   - 2026-11-10 01:00: now uses Recorder, Op, Pointer, were _Recorder, _Op, _Pointer
//     (C120a, Q314)
func (r *Recorder) _Enlist(
	size int,
	value *structpb.Value,
	parts ...any,
) {
	if size == 1 {
		r.Op(
			ADD,
			Pointer(parts[:len(parts)-1]...),
			_Listed(value),
		)

		return
	}

	r.Op(
		ADD,
		Pointer(parts...),
		value,
	)
}

// _Delist reports that an element left an array of the graph. size is how many
// elements the array holds now, the leaver gone.
//
// parts is the path of the element: the array's path, then its index. When
// none is left, protojson leaves the array out, so the change removes the
// array itself, at parts without its last part (RFC 6902, section 4.2).
//
// Callers hold the lock and have seen that a watch is attached.
//
// Revisions:
//   - 2026-11-02 00:54: initial creation, from _Removed
//   - 2026-11-10 01:00: now uses Recorder, Op, Pointer, were _Recorder, _Op, _Pointer
//     (C120a, Q314)
func (r *Recorder) _Delist(
	size int,
	parts ...any,
) {
	if size == 0 {
		parts = parts[:len(parts)-1]
	}

	r.Op(
		REMOVE,
		Pointer(parts...),
		nil,
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
//   - 2026-11-02 00:54: adds the node through _Enlist
//   - 2026-11-05 00:27: adds it through _Added (C45, Q18)
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) _Running(name string) *_Callee {
	callee, known := r.callees[name]
	if known {
		r._Status(
			callee,
			workflowpb.Status_STATUS_RUNNING,
		)

		return callee
	}

	return r._Added(
		name,
		workflowpb.Status_STATUS_RUNNING,
	)
}

// _Added adds the node of the function name, which the graph lacks, in its
// place and with status, and returns the function's record.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation, out of _Running, so a node can be
//     added with a status other than RUNNING (C45, Q18)
//   - 2026-11-10 01:00: now uses Recorder, Watch, were _Recorder, watch (C120a, Q314)
func (r *Recorder) _Added(
	name string,
	status workflowpb.Status,
) *_Callee {
	var (
		node = &workflowpb.Node{
			Name:   name,
			Status: status,
		}
		callee = &_Callee{
			node: node,
		}
	)

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

	if r.Watch == nil {
		return callee
	}

	r._Enlist(
		len(r.names),
		_Valued(node),
		FUNCTIONS,
		idx,
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
//   - 2026-11-02 08:25: calls _Statused, renamed from _Status
//   - 2026-11-10 01:00: now uses Recorder, Watch, Op, Pointer, were _Recorder, watch, _Op,
//     _Pointer (C120a, Q314)
func (r *Recorder) _Status(
	callee *_Callee,
	status workflowpb.Status,
) {
	callee.node.Status = status

	if r.Watch == nil {
		return
	}

	r.Op(
		ADD,
		Pointer(
			FUNCTIONS,
			r._Place(callee.node.GetName()),
			STATUS,
		),
		_Statused(status),
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
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) _Moved(
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
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) _Left(
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
//   - 2026-11-02 00:54: removes the thread through _Delist
//   - 2026-11-10 01:00: now uses Recorder, Watch, were _Recorder, watch (C120a, Q314)
func (r *Recorder) _Removed(
	callee *_Callee,
	idx int,
) {
	if r.Watch == nil {
		return
	}

	var (
		node  = callee.node
		place = r._Place(node.GetName())
	)

	r._Delist(
		len(node.Threads),
		FUNCTIONS,
		place,
		THREADS,
		idx,
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
//   - 2026-11-02 00:54: adds the thread through _Enlist
//   - 2026-11-10 01:00: now uses Recorder, Watch, were _Recorder, watch (C120a, Q314)
func (r *Recorder) _Joined(
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

	if r.Watch == nil {
		return
	}

	var (
		place = r._Place(node.GetName())
		lane  = structpb.NewStringValue(thread.name)
	)

	r._Enlist(
		len(node.Threads),
		lane,
		FUNCTIONS,
		place,
		THREADS,
		APPEND,
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
//   - 2026-11-02 00:54: adds the edge through _Enlist
//   - 2026-11-10 01:00: now uses Recorder, Watch, were _Recorder, watch (C120a, Q314)
func (r *Recorder) _Edge(
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

	if r.Watch == nil {
		return
	}

	r._Enlist(
		len(r.edges),
		_Valued(edge),
		CALLS,
		idx,
	)
}

// _Place is the position of the function name's node in the graph.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) _Place(name string) int {
	idx, _ := slices.BinarySearch(
		r.names,
		name,
	)

	return idx
}

// _Fault is the error a panicking watcher left, or nil.
//
// Not under the guard: fault is written inside emit, and read after the run
// ended, so the happens-before edge is the goroutine join (Wait, or the
// caller's own check in Start).
//
// Revisions:
//   - 2026-10-24 16:10: initial creation
//   - 2026-11-10 01:00: now uses Recorder, were _Recorder (C120a, Q314)
func (r *Recorder) _Fault() error {
	return r.fault
}

// _Standing is the graph as the recorder holds it now, which is what Drawn
// answers: the recorder is the live graph, drawn at any moment.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 01:00: now uses Drawn, Recorder, were _Drawn, _Recorder (C120a, Q314)
func (r *Recorder) _Standing() *workflowpb.Graph {
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

// Pointer is the JSON Pointer that parts name, each a member or an index.
//
// Built in a buffer on the stack, an index appended as digits, so the one
// allocation is the string it returns. A member goes in as it is. RFC 6901,
// section 3 escapes "~" as ~0 and "/" as ~1, but no name is ever part of a
// path: the parts are constants and indexes, and names go into values. So the
// parts need no escaping. A part of any other type adds only its slash.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-02 16:18: appends to a stack buffer rather than a Builder
//   - 2026-11-02 16:35: says why the parts need no escaping
//   - 2026-11-10 00:25: renamed from _Pointer, as a test in tests/ needs it (C120a, Q314)
func Pointer(parts ...any) string {
	var stack [64]byte

	held := stack[:0]

	for _, part := range parts {
		held = append(
			held,
			SLASH...,
		)

		switch named := part.(type) {
		case int:
			held = strconv.AppendInt(
				held,
				int64(named),
				DECIMAL,
			)
		case string:
			held = append(
				held,
				named...,
			)
		}
	}

	return string(held)
}

// _Statused is status as JSON writes it: by the enum's name.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-11-02 08:25: renamed from _Status; the method keeps that name
func _Statused(status workflowpb.Status) *structpb.Value {
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
