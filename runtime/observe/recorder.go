package observe

import (
	"sort"
	"sync"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime/scheduler"
)

// _Node is what one function is doing, before it becomes a message.
//
// Its own shape rather than the generated Node, because a node has to be found
// again by function and thread to be updated, and the thread it belongs to is
// not a field of the generated message - it is which list the message is in.
type _Node struct {
	thread  int32
	name    string
	status  workflowpb.Status
	attempt int32
	failure string
}

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
	nodes map[_Where]*_Node
	order []_Where
	lanes map[int32]bool
	cause *workflowpb.Cause
}

// _Where is a node's identity: which function, on which thread.
type _Where struct {
	thread int32
	name   string
}

// _NewRecorder returns a recorder that has been told nothing.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func _NewRecorder() *_Recorder {
	return &_Recorder{
		nodes: make(map[_Where]*_Node),
		lanes: make(map[int32]bool),
	}
}

// Started records that a function has begun on a thread, on the attempt given.
//
// A function reported again is the same node on a later attempt, not a second
// node. That is what makes repeat(step, 3) one entry advancing 1, 2, 3.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func (r *_Recorder) Started(thread int32, name string, attempt int32) {
	r.guard.Lock()
	defer r.guard.Unlock()

	node := r._At(thread, name)

	node.status = workflowpb.Status_STATUS_RUNNING
	node.attempt = attempt
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
func (r *_Recorder) Ended(thread int32, name string, err error) {
	r.guard.Lock()
	defer r.guard.Unlock()

	node := r._At(thread, name)

	node.status = _Became(err)
	node.failure = _Why(err)

	r._Blame(node)
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
func (r *_Recorder) _Blame(node *_Node) {
	if r.cause != nil || node.status != workflowpb.Status_STATUS_FAILED {
		return
	}

	r.cause = &workflowpb.Cause{
		Thread:   node.thread,
		Function: node.name,
		Failure:  node.failure,
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
func (r *_Recorder) _Seed(graph *workflowpb.Graph) {
	r.guard.Lock()
	defer r.guard.Unlock()

	for slot, thread := range graph.GetThreads() {
		lane := int32(slot)

		r._Lane(lane)

		for _, step := range thread.GetSteps() {
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
// A fork names no function - it says a thread exists, and whatever runs there is
// that thread's own steps to declare. Recording the lane is still worth doing:
// a graph may fork a thread it never describes, and an empty lane is what phase
// 7 means by one a run never fills.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func (r *_Recorder) _Place(thread int32, step *workflowpb.Step) {
	if fork := step.GetFork(); fork != nil {
		r._Lane(fork.GetThread())

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
func (r *_Recorder) _Lane(index int32) {
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

// _Names is the function a step runs, or empty for a step that runs none.
//
// A wrapper names its function as surely as a call does: a Repeat of fetch runs
// fetch, and a graph that placed the call but not the repeat would put the same
// function in two places depending on how it was written.
//
// Revisions:
//   - 2026-09-20 11:55: initial creation
func _Names(step *workflowpb.Step) string {
	switch {
	case step.GetCall() != nil:
		return step.GetCall().GetFunction()
	case step.GetRepeat() != nil:
		return step.GetRepeat().GetFunction()
	case step.GetRetry() != nil:
		return step.GetRetry().GetFunction()
	case step.GetTimeout() != nil:
		return step.GetTimeout().GetFunction()
	}

	return ""
}

// _Threads is every thread's nodes, in the order each was first heard of.
//
// Revisions:
//   - 2026-09-20 01:39: initial creation
func (r *_Recorder) _Threads() []*workflowpb.Thread {
	r.guard.Lock()
	defer r.guard.Unlock()

	lanes := make(map[int32]*workflowpb.Thread)

	var numbers []int32

	for index := range r.lanes {
		lanes[index] = &workflowpb.Thread{Index: index}
		numbers = append(numbers, index)
	}

	for _, where := range r.order {
		node := r.nodes[where]

		lane, known := lanes[where.thread]
		if !known {
			lane = &workflowpb.Thread{Index: where.thread}
			lanes[where.thread] = lane
			numbers = append(numbers, where.thread)
		}

		lane.Nodes = append(lane.Nodes, &workflowpb.Node{
			Function: node.name,
			Status:   node.status,
			Attempt:  node.attempt,
			Failure:  node.failure,
		})
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
func (r *_Recorder) _At(thread int32, name string) *_Node {
	where := _Where{thread: thread, name: name}

	node, known := r.nodes[where]
	if known {
		return node
	}

	node = &_Node{
		thread: thread,
		name:   name,
		status: workflowpb.Status_STATUS_PENDING,
	}

	r.nodes[where] = node
	r.order = append(r.order, where)
	r.lanes[thread] = true

	return node
}
