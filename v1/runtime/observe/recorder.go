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
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// NONE is the position of no call, among the calls a thread is in.
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
// nodes holds each function the run called, by name, and names those names
// sorted, which is each node's place in the graph; edges holds the calls,
// sorted by caller and then callee. running is what each thread is in the
// middle of: the calls it has started and not ended, newest last, so the newest
// is the caller of whatever starts next, the function the thread is executing,
// and the first an end can close. newest is, for each function, the call its
// node shows, and made counts the calls started, which is what numbers them.
// cause is the first failure, the one that ended the run. status and ending are
// the run's own, as its graph reports them: running, with no cause, until it
// ends; then how it ended and, when it failed, why.
//
// arrival counts the times a thread arrived at a node, and arrived is each
// thread's number at the node it is executing. A node lists its threads in the
// order they arrived, so their numbers are sorted, and a thread that leaves is
// found by a binary search rather than by reading the names one by one, which
// cost the square of the threads in one function.
//
// watch is told each step's change, and ops collects it while the step is
// folded; nil when nothing watches, so a run nobody watches builds none. emit
// is held across a step's fold and its telling, so changes arrive in the order
// they were made, and the graph a watcher asks for matches the change it is
// told. watch is set before the run starts and never changes.
type _Recorder struct {
	guard   sync.Mutex
	nodes   map[string]*workflowpb.Node
	names   []string
	edges   []*workflowpb.Edge
	running map[string][]*_Call
	newest  map[string]int
	made    int
	cause   *workflowpb.Cause
	status  workflowpb.Status
	ending  *workflowpb.Cause

	arrival int
	arrived map[string]int

	watch func(change *workflowpb.Change)
	ops   []*patchpb.Operation
	emit  sync.Mutex
}

// _Call is one call a thread has started and not ended: the function, and
// which call of the run it is.
type _Call struct {
	name    string
	ordinal int
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
func _NewRecorder() *_Recorder {
	return &_Recorder{
		nodes:   make(map[string]*workflowpb.Node),
		running: make(map[string][]*_Call),
		newest:  make(map[string]int),
		arrived: make(map[string]int),
	}
}

// Started records a call a thread began, and tells the watcher what it
// changed.
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
func (r *_Recorder) Started(thread string, line *scheduler.Line) {
	r._Step(func() {
		r._Start(thread, line)
	})
}

// _Start folds a call a thread began into the graph.
//
// A line that calls nothing - a join, a sleep, a cancel - is no call. A later
// attempt of a repeat or a retry is the call already running, not another, so
// repeat(3, step) is one call. Anything else is a new call: its function's
// node runs, shows this call from now on, and gains an edge from the call the
// thread is in the middle of, when it is in one; and the thread moves to the
// node it is now executing.
//
// A spawn's call runs on the thread it started and ends there, so it is that
// thread's first call - and its caller is whatever the spawning thread is in
// the middle of.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 01:31: initial creation
//   - 2026-10-02 13:12: returns no change, nothing being told one
//   - 2026-10-02 15:34: a call of a function's node, with an edge from the
//     thread's innermost call
//   - 2026-10-02 16:33: moves the thread to the node it now executes
func (r *_Recorder) _Start(thread string, line *scheduler.Line) {
	if line.Name == "" {
		return
	}

	advancing := line.Attempt > ONCE && r._Newest(thread, line.Name) != NONE
	if advancing {
		return
	}

	caller := r._Innermost(thread)

	lane := thread
	if line.Child != "" {
		lane = line.Child
	}

	executing := r._Innermost(lane)

	r.made++
	r.running[lane] = append(r.running[lane], &_Call{name: line.Name, ordinal: r.made})
	r.newest[line.Name] = r.made

	r._Running(line.Name)
	r._Moved(lane, executing, line.Name)

	if caller != "" {
		r._Edge(caller, line.Name)
	}
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
func (r *_Recorder) Ended(thread string, name string, err error) {
	r._Step(func() {
		r._End(thread, name, err)
	})
}

// _End folds a call's end into the graph.
//
// It closes the newest call to name's function on that thread: calls on one
// thread start and end in order, so the newest is the one that ended. The
// thread moves back to whatever it is executing now, and the function's node
// takes the status only while this is its newest call, since a node shows its
// newest call. An empty name is a join, a sleep or a cancel, which call
// nothing, and an end that matches no call is one this recorder was never told
// began; neither changes anything.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 01:31: initial creation
//   - 2026-10-02 13:12: returns no change, nothing being told one
//   - 2026-10-02 15:34: closes the thread's newest call to the function, and
//     sets its node only while it is the function's newest
//   - 2026-10-02 16:33: moves the thread back to what it executes now
func (r *_Recorder) _End(thread string, name string, err error) {
	if name == "" {
		return
	}

	executing := r._Innermost(thread)

	ended := r._Close(thread, name)
	if ended == nil {
		return
	}

	r._Moved(thread, executing, r._Innermost(thread))

	status := _Became(err)

	if r.newest[name] == ended.ordinal {
		r._Status(name, status)
	}

	r._Blame(name, status, err)
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

		r._Op(ADD, _Pointer(STATUS), _Status(r.status))
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
func (r *_Recorder) _Finished(status workflowpb.Status, cause *workflowpb.Cause) {
	r._Step(func() {
		r.status = status
		r.ending = cause

		if r.watch == nil {
			return
		}

		r._Op(ADD, _Pointer(STATUS), _Status(status))

		if cause != nil {
			r._Op(ADD, _Pointer(CAUSE), _Valued(cause))
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
// Revisions:
//   - 2026-10-02 16:33: initial creation
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

	if len(ops) > 0 {
		r.watch(&workflowpb.Change{Operations: ops})
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
func (r *_Recorder) _Op(op string, path string, value *structpb.Value) {
	r.ops = append(r.ops, &patchpb.Operation{Op: op, Path: path, Value: value})
}

// _Running makes the node of the function name running, adding it in its
// place when the run first calls it.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation, from _Node
func (r *_Recorder) _Running(name string) {
	_, known := r.nodes[name]
	if known {
		r._Status(name, workflowpb.Status_STATUS_RUNNING)

		return
	}

	node := &workflowpb.Node{Name: name, Status: workflowpb.Status_STATUS_RUNNING}
	r.nodes[name] = node

	idx, _ := slices.BinarySearch(r.names, name)
	r.names = slices.Insert(r.names, idx, name)

	if r.watch == nil {
		return
	}

	if len(r.names) == 1 {
		r._Op(ADD, _Pointer(FUNCTIONS), _Listed(_Valued(node)))

		return
	}

	r._Op(ADD, _Pointer(FUNCTIONS, idx), _Valued(node))
}

// _Status sets the status of the function name's node.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func (r *_Recorder) _Status(name string, status workflowpb.Status) {
	r.nodes[name].Status = status

	if r.watch == nil {
		return
	}

	r._Op(ADD, _Pointer(FUNCTIONS, r._Place(name), STATUS), _Status(status))
}

// _Moved moves thread from the node of the function it was executing to the
// node of the one it executes now. Either may be empty: a thread starting
// executes nothing before, and one whose last call ended nothing after.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func (r *_Recorder) _Moved(thread string, from string, to string) {
	if from == to {
		return
	}

	if from != "" {
		r._Left(thread, from)
	}

	if to != "" {
		r._Joined(thread, to)
	}
}

// _Left takes thread off the threads executing the function name.
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
func (r *_Recorder) _Left(thread string, name string) {
	node := r.nodes[name]

	idx, found := slices.BinarySearchFunc(
		node.Threads,
		r.arrived[thread],
		func(lane string, ticket int) int {
			return cmp.Compare(r.arrived[lane], ticket)
		},
	)
	if !found {
		return
	}

	node.Threads = slices.Delete(node.Threads, idx, idx+1)
	delete(r.arrived, thread)

	if r.watch == nil {
		return
	}

	place := r._Place(name)

	if len(node.Threads) == 0 {
		r._Op(REMOVE, _Pointer(FUNCTIONS, place, THREADS), nil)

		return
	}

	r._Op(REMOVE, _Pointer(FUNCTIONS, place, THREADS, idx), nil)
}

// _Joined adds thread to the threads executing the function name.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 21:02: numbers the arrival, so the thread is found again by it
func (r *_Recorder) _Joined(thread string, name string) {
	node := r.nodes[name]
	node.Threads = append(node.Threads, thread)

	r.arrival++
	r.arrived[thread] = r.arrival

	if r.watch == nil {
		return
	}

	place := r._Place(name)
	lane := structpb.NewStringValue(thread)

	if len(node.Threads) == 1 {
		r._Op(ADD, _Pointer(FUNCTIONS, place, THREADS), _Listed(lane))

		return
	}

	r._Op(ADD, _Pointer(FUNCTIONS, place, THREADS, APPEND), lane)
}

// _Edge records that caller called callee, once however often it does.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-10-02 16:33: keeps the edges in order, adding each in its place
func (r *_Recorder) _Edge(caller string, callee string) {
	edge := &workflowpb.Edge{Caller: caller, Callee: callee}

	idx, found := slices.BinarySearchFunc(r.edges, edge, _Order)
	if found {
		return
	}

	r.edges = slices.Insert(r.edges, idx, edge)

	if r.watch == nil {
		return
	}

	if len(r.edges) == 1 {
		r._Op(ADD, _Pointer(CALLS), _Listed(_Valued(edge)))

		return
	}

	r._Op(ADD, _Pointer(CALLS, idx), _Valued(edge))
}

// _Place is the position of the function name's node in the graph.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func (r *_Recorder) _Place(name string) int {
	idx, _ := slices.BinarySearch(r.names, name)

	return idx
}

// _Newest is the position of the newest call to name that thread is in, or
// NONE.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
func (r *_Recorder) _Newest(thread string, name string) int {
	calls := r.running[thread]

	for idx := len(calls) - 1; idx >= 0; idx-- {
		if calls[idx].name == name {
			return idx
		}
	}

	return NONE
}

// _Innermost is the function of the newest call thread is in, or empty when it
// is in none.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
func (r *_Recorder) _Innermost(thread string) string {
	calls := r.running[thread]
	if len(calls) == 0 {
		return ""
	}

	return calls[len(calls)-1].name
}

// _Close takes the newest call to name off thread's calls, and returns it, or
// nil when thread is in none.
//
// A thread left in no call is forgotten, so a long run holds only the threads
// still going.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
func (r *_Recorder) _Close(thread string, name string) *_Call {
	idx := r._Newest(thread, name)
	if idx == NONE {
		return nil
	}

	calls := r.running[thread]
	ended := calls[idx]

	left := slices.Delete(calls, idx, idx+1)
	if len(left) == 0 {
		delete(r.running, thread)

		return ended
	}

	r.running[thread] = left

	return ended
}

// _Blame remembers the first call to fail, which is the one that ended the
// run: the function it called, and what it said.
//
// By construction rather than by luck: a thread stopped because something else
// failed reports a cancellation, not a failure, so the first failure is the
// innermost call of the thread that failed. Two threads can fail before
// either cancellation lands - and then this names the first the recorder
// heard of, which the scheduler may not agree was the outcome. Both are real
// failures and both are reported; what can differ is which is called the one
// that ended things.
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
func (r *_Recorder) _Blame(name string, status workflowpb.Status, err error) {
	if r.cause != nil || status != workflowpb.Status_STATUS_FAILED {
		return
	}

	r.cause = &workflowpb.Cause{
		Function: name,
		Failure:  _Why(err),
	}
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
func (r *_Recorder) _Because(status workflowpb.Status, failure string) *workflowpb.Cause {
	if status != workflowpb.Status_STATUS_FAILED {
		return nil
	}

	found := r._Cause()
	if found != nil {
		return found
	}

	return &workflowpb.Cause{Failure: failure}
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

	graph := &workflowpb.Graph{Status: r.status}

	if r.ending != nil {
		graph.Cause = proto.CloneOf(r.ending)
	}

	for _, name := range r.names {
		graph.Functions = append(graph.Functions, proto.CloneOf(r.nodes[name]))
	}

	for _, edge := range r.edges {
		graph.Calls = append(graph.Calls, proto.CloneOf(edge))
	}

	return graph
}

// _Order orders two edges as a graph lists them: by caller, then callee.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Order(first *workflowpb.Edge, second *workflowpb.Edge) int {
	return cmp.Or(
		strings.Compare(first.GetCaller(), second.GetCaller()),
		strings.Compare(first.GetCallee(), second.GetCallee()),
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
	return structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{value}})
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
