package observe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// _Recorder is what every function in one run is doing.
//
// One per run, created by Start and held on the entry. It implements the
// scheduler's Reporter, which is how a run tells it anything: the store cannot
// ask, because it holds no run.
//
// A function is keyed by name and thread together. The same function spawned on
// two threads is two nodes, because a report of one lane saying "worker
// succeeded" while the other is still going would be a report of neither.
type _Recorder struct {
	guard sync.Mutex
	nodes map[_Where]*workflowpb.Node
	order []_Where
	lanes map[string]bool
	cause *workflowpb.Cause

	print func(string)
	dir   string
	logs  *Log
}

// _Where is a node's identity: which function, on which thread.
//
// This is why a node needs no shape of its own. A generated Node carries no
// thread - which thread it belongs to is which list it ends up in - and that
// was the whole reason for a second struct. The key already holds it, so the
// message itself is what the recorder keeps.
type _Where struct {
	thread string
	name   string
}

// _NewRecorder returns a recorder that has been told nothing, and that hands
// what a script prints to standard error until a host says otherwise.
//
// Standard error because that is where the interpreter's own default writes,
// so a host that says nothing sees what it always saw.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 09:46: prints to standard error by default
func _NewRecorder() *_Recorder {
	return &_Recorder{
		nodes: make(map[_Where]*workflowpb.Node),
		lanes: make(map[string]bool),
		print: _Stderr,
	}
}

// _Stderr writes one printed line where the interpreter's default would.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
func _Stderr(msg string) {
	_, err := fmt.Fprintln(os.Stderr, msg)
	if err != nil {
		// Nothing further to tell: the channel that failed is the one a
		// failure would be told on.
		return
	}
}

// Printed hands a script's line to whatever the host asked for, and to the
// run's own file when it has one.
//
// Not recorded in the snapshot. A Workflow carries statuses, and a host that
// asked for logs knows where they are: WithLogs says which directory, and a
// run's file is its id with .log on the end. That rule is the whole of what a
// poller needs, which is why the message gained no field.
//
// The printer is handed the line as the script wrote it and the file gets the
// lane in front, because a printer is a host's own stream and a file is a
// transcript of a concurrent run.
//
// logs is written before the evaluation starts and read only by threads that
// evaluation creates, so no lock covers it.
//
// Revisions:
//   - 2026-09-21 09:46: initial creation
//   - 2026-09-21 16:42: writes the run's own file too
func (r *_Recorder) Printed(thread string, msg string) {
	r.print(msg)

	if r.logs != nil {
		r.logs.Printed(thread, msg)
	}
}

// _Open gives this run its own file, if a host asked for one.
//
// Named for the run rather than the script, because two runs of one artifact
// are two transcripts and an id is what tells them apart.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (r *_Recorder) _Open(id string) error {
	if r.dir == "" {
		return nil
	}

	made, err := NewLog(filepath.Join(r.dir, id+SUFFIX))
	if err != nil {
		return err
	}

	r.logs = made

	return nil
}

// _Close finishes the run's file, and says so if it could not.
//
// A run whose transcript could not be finished is a run a caller should hear
// about, so this is reported rather than dropped - unlike a failure to write
// one line, which has nowhere to go.
//
// Revisions:
//   - 2026-09-21 16:42: initial creation
func (r *_Recorder) _Close() error {
	if r.logs == nil {
		return nil
	}

	return r.logs.Close()
}

// _Resolve is the name to report for a function on a thread.
//
// An anonymous function has none of its own, and this is where the graph
// supplies one - but only where the graph can. A fork names a lane and that
// lane declares what runs there, so a spawned lambda has exactly one candidate.
// A wrapped one runs on the lane that called it, and a lane may hold several
// functions, so there is nothing to choose between them and nothing is chosen.
//
// Empty rather than "lambda", which reads like a function of that name. A
// reader sees a status with no name, which is what is true.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 20:53: initial creation
func (r *_Recorder) _Resolve(thread string, name string) string {
	if name != scheduler.LAMBDA {
		return name
	}

	var found string

	for where := range r.nodes {
		if where.thread != thread || where.name == "" {
			continue
		}

		if found != "" {
			return ""
		}

		found = where.name
	}

	return found
}

// Started records that a function has begun on a thread, on the attempt given.
//
// A function reported again is the same node on a later attempt, not a second
// node. That is what makes repeat(step, 3) one entry advancing 1, 2, 3.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-20 20:53: resolves an anonymous function against the graph
func (r *_Recorder) Started(thread string, name string, attempt int32) {
	r.guard.Lock()
	defer r.guard.Unlock()

	node := r._At(thread, r._Resolve(thread, name))

	node.Status = workflowpb.Status_STATUS_RUNNING
	node.Attempt = attempt
}

// Ended records how a function finished.
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
func (r *_Recorder) Ended(thread string, name string, err error) {
	r.guard.Lock()
	defer r.guard.Unlock()

	where := _Where{thread: thread, name: r._Resolve(thread, name)}

	node := r._At(where.thread, where.name)

	node.Status = _Became(err)
	node.Failure = _Why(err)

	r._Blame(&where, node)
}

// _Blame remembers the first function to fail, which is the one that ended the
// run.
//
// By construction rather than by luck: a thread stopped because something else
// failed reports a cancellation, not a failure, so normally exactly one node is
// ever FAILED. Two can be, when both failed before either cancellation landed -
// and then this names the first the recorder heard of, which the scheduler may
// not agree was the outcome. Both are real failures and both are reported; what
// can differ is which is called the one that ended things.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
//   - 2026-09-21 01:21: takes the node's identity, which the key holds, so the
//     message needs no thread of its own
//   - 2026-09-21 08:09: reached through a pointer, as every struct here is
func (r *_Recorder) _Blame(where *_Where, node *workflowpb.Node) {
	if r.cause != nil || node.GetStatus() != workflowpb.Status_STATUS_FAILED {
		return
	}

	r.cause = &workflowpb.Cause{
		Thread:   where.thread,
		Function: where.name,
		Failure:  node.GetFailure(),
	}
}

// _Because is the cause to report for a run that ended this way, or nil.
//
// Nil unless the run failed, so a host tests the pointer rather than the enum.
//
// A run can fail with no node blamed - the entry point itself raising before
// any function was reported, or a failure the scheduler saw and no thread did.
// Then the run's own text stands in, on the spine, with no function named:
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

// _Cause is the node whose failure ended the run, or nil.
//
// Revisions:
//   - 2026-09-20 11:39: initial creation
func (r *_Recorder) _Cause() *workflowpb.Cause {
	r.guard.Lock()
	defer r.guard.Unlock()

	return r.cause
}

// _Seed enters every function a graph declares as pending.
//
// A graph is a floor, not a ceiling: it may add functions that have not
// happened, and it may never remove or rename one that did. So this only ever
// enters what is missing, and a later report about the same function overwrites
// the pending entry rather than being refused.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-20 18:40: numbers Graph lanes by list slot; GraphThread has
//     no index
//   - 2026-09-21 00:59: takes each thread's own id, which the thread now
//     carries, instead of numbering lanes by list slot
//   - 2026-09-21 23:53: a thread's own function is its first step, so placing
//     the steps is the whole of it
func (r *_Recorder) _Seed(graph *workflowpb.Graph) {
	r.guard.Lock()
	defer r.guard.Unlock()

	for _, thread := range graph.GetThreads() {
		lane := thread.GetId()

		r._Lane(lane)

		// Every step, the first included: what a thread runs is its first step,
		// so placing the steps places it without a case of its own.
		for _, step := range thread.GetStatic().GetSteps() {
			r._Place(lane, step)
		}
	}

	// Declaration after placement, and only for what placement missed. A
	// function the graph both declares and runs somewhere would otherwise be
	// entered twice - once on its own lane and once on the spine - and the
	// spine copy would stay pending for ever, because nothing runs there.
	for _, fn := range graph.GetFunctions() {
		if r._Placed(fn.GetName()) {
			continue
		}

		r._At(scheduler.SPINE, fn.GetName())
	}
}

// _Place enters whatever function a step names, on the thread the graph runs it
// on.
//
// A spawn names no function - it says a thread exists, and whatever runs there
// is that thread's own entry to declare. Recording the lane is still worth
// doing: a graph may spawn a thread it never describes, and an empty lane is
// what phase 7 means by one a run never fills.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
//   - 2026-09-21 01:32: a Fork names the thread it starts
func (r *_Recorder) _Place(thread string, step *workflowpb.Step) {
	if spawned := _Spawned(step); spawned != "" {
		r._Lane(spawned)

		return
	}

	named := _Names(step)
	if named == "" {
		return
	}

	r._At(thread, named)
}

// _Lane records that a thread exists, whether or not anything is known to run
// on it.
//
// Lanes are kept apart from nodes because a lane with no nodes cannot be
// inferred from its nodes.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func (r *_Recorder) _Lane(index string) {
	r.lanes[index] = true
}

// _Placed reports whether any thread already runs this function.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func (r *_Recorder) _Placed(name string) bool {
	for where := range r.nodes {
		if where.name == name {
			return true
		}
	}

	return false
}

// _Spawned is the thread a fork step starts, or empty for any other step.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Spawned(step *workflowpb.Step) string {
	return step.GetFork().GetThread()
}

// _Live is an empty running thread under this id.
//
// A snapshot always reports the live half, even for a thread nothing has run
// on yet, so a host reads one arm rather than testing which it was given.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Live(id string) *workflowpb.Thread {
	return &workflowpb.Thread{
		Id:    id,
		State: &workflowpb.Thread_Live{Live: &workflowpb.Live{}},
	}
}

// _Names is the function a step runs, or empty for a step that runs none.
//
// A wrapper names its function as surely as a call does: a Repeat of fetch runs
// fetch, and a graph that placed the call but not the repeat would put the same
// function in two places depending on how it was written.
//
// Every one of them carries a Call, so this reads one field through a different
// wrapper rather than four fields that spell the same thing four ways.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func _Names(step *workflowpb.Step) string {
	switch {
	case step.GetCall() != nil:
		return step.GetCall().GetFunction()
	case step.GetRepeat() != nil:
		return step.GetRepeat().GetCall().GetFunction()
	case step.GetRetry() != nil:
		return step.GetRetry().GetCall().GetFunction()
	case step.GetTimeout() != nil:
		return step.GetTimeout().GetCall().GetFunction()
	}

	return ""
}

// _Threads is every thread's nodes, in the order each was first heard of.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 08:09: clones through the typed clone, so nothing is
//     silently dropped on a failed assertion that cannot fail
func (r *_Recorder) _Threads() []*workflowpb.Thread {
	r.guard.Lock()
	defer r.guard.Unlock()

	lanes := make(map[string]*workflowpb.Thread)

	var numbers []string

	for index := range r.lanes {
		lanes[index] = _Live(index)
		numbers = append(numbers, index)
	}

	for _, where := range r.order {
		node := r.nodes[where]

		lane, known := lanes[where.thread]
		if !known {
			lane = _Live(where.thread)
			lanes[where.thread] = lane
			numbers = append(numbers, where.thread)
		}

		live := lane.GetLive()

		// Cloned, not aliased. The recorder keeps writing to its nodes after a
		// snapshot is handed out, so sharing one would let a finished report
		// change under whoever is reading it. A clone rather than a copy of
		// the fields, because a field added to Node later would be dropped by
		// a copy and nothing would say so.
		live.Nodes = append(live.Nodes, proto.CloneOf(node))
	}

	sort.Slice(numbers, func(i, j int) bool {
		return numbers[i] < numbers[j]
	})

	threads := make([]*workflowpb.Thread, 0, len(numbers))

	for _, number := range numbers {
		threads = append(threads, lanes[number])
	}

	return threads
}

// _At returns the node for a function on a thread, entering it as pending if
// nothing has been said about it yet.
//
// Pending is the right entry state for a node nobody has reported: without a
// graph nothing reaches here until a function starts, and with one every seeded
// function is genuinely pending until it does.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
//   - 2026-09-21 01:21: keeps the generated Node rather than a shape of its own
func (r *_Recorder) _At(thread string, name string) *workflowpb.Node {
	where := _Where{thread: thread, name: name}

	node, known := r.nodes[where]
	if known {
		return node
	}

	node = &workflowpb.Node{
		Function: name,
		Status:   workflowpb.Status_STATUS_PENDING,
	}

	r.nodes[where] = node
	r.order = append(r.order, where)
	r.lanes[thread] = true

	return node
}
