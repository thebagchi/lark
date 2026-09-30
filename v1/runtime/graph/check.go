package graph

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

var (
	// ErrLive is returned for a graph whose thread carries the running half. A
	// Graph describes what will happen; progress belongs to a Workflow.
	ErrLive = errors.New("a graph carries no progress")

	// ErrParentage is returned for a thread id that does not descend from the
	// thread that forks it.
	//
	// This is the worry a hierarchical id gives
	// it. §9 removed the id so a user interface could not send an index that
	// disagreed with list position; an id disagreeing with its parentage is
	// checkable where an inconsistent index was not, because an id has a rule
	// and a position does not.
	ErrParentage = errors.New("a thread id does not name its parent")

	// ErrTwoSpines is returned when more than one thread claims the spine's
	// id. The spine is thread_0 and there is one of those, since what runs
	// there is the artifact's entry point.
	ErrTwoSpines = errors.New("a graph has one entry point")

	// ErrArity is returned for a Call passing more values than the function it
	// names has parameters.
	//
	// args is positional and lines up with params index for index, so a count
	// that cannot fit is a graph that will fail when it runs. Caught here
	// rather than reaching Starlark as an error against a line in generated
	// source, which names nothing a reader can act on.
	ErrArity = errors.New("a call passes more arguments than the function takes")

	// ErrNotForked is returned for a join or a cancel naming a thread that the
	// thread holding it did not fork before it.
	//
	// A generated script names a thread by the handle its fork bound, in the
	// function that forked it, so a join anywhere else - or before the fork -
	// names a variable that does not exist. Caught here rather than as an
	// undefined name in generated source, which points at nothing a reader
	// wrote.
	ErrNotForked = errors.New("a thread is joined or cancelled where it was not forked")

	// ErrForkCall is returned for a fork whose func is not the call its thread
	// runs. The call is said twice so each side reads alone, and two copies are
	// worth having only while they agree.
	ErrForkCall = errors.New("a fork's call is not what its thread runs")

	// ErrUnresolved is returned for a parameter a call cannot see: not a param
	// of the function making the call, and not a function, constant or argument
	// the graph declares. A constant passes values only, since it is computed
	// before anything a parameter could name is bound.
	ErrUnresolved = errors.New("a parameter names nothing the call can see")
)

// Check is what a graph must satisfy before anything generates from it.
//
// Exported because a host taking a graph from a user interface needs it, and
// that host is not this package. Emit does not call it: generating is the
// caller's decision and this is the caller's check, so a host that has already
// validated does not pay twice.
//
// Distinct is part of it, and was not until a host came to call this and had
// to be told to call both. A sentence saying "what a graph must satisfy" is
// either all of it or a trap: two functions named greet passed here and failed
// only for whoever remembered the second call. Distinct stays exported, for a
// user interface checking one edit rather than a whole graph.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 16:25: includes Distinct, which its own first line had always
//     claimed
//   - 2026-09-30 00:44: refuses a join or a cancel of a thread not forked
//     there, a fork whose call is not its thread's, and a parameter its call
//     cannot see
func Check(graph *workflowpb.Graph) error {
	err := Distinct(graph)
	if err != nil {
		return err
	}

	err = _Spines(graph)
	if err != nil {
		return err
	}

	err = _Parentage(graph)
	if err != nil {
		return err
	}

	err = _Arity(graph)
	if err != nil {
		return err
	}

	err = _Waits(graph)
	if err != nil {
		return err
	}

	err = _Forks(graph)
	if err != nil {
		return err
	}

	return _ScopeOf(graph)._Resolved(graph)
}

// _Waits checks that every join and cancel names threads forked earlier on the
// thread holding it.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Waits(graph *workflowpb.Graph) error {
	for _, thread := range graph.GetThreads() {
		err := _Forked(thread)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Forked checks one thread's joins and cancels against the forks before them.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Forked(thread *workflowpb.Thread) error {
	forked := make(map[string]bool)

	for _, step := range thread.GetStatic().GetSteps() {
		if step.GetFork() != nil {
			forked[step.GetFork().GetThread()] = true

			continue
		}

		for _, id := range _Awaited(step) {
			if !forked[id] {
				return fmt.Errorf("%s on %s: %w", id, thread.GetId(), ErrNotForked)
			}
		}
	}

	return nil
}

// _Awaited is the threads a join or a cancel names, or none for any other step.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Awaited(step *workflowpb.Step) []string {
	switch {
	case step.GetJoin() != nil:
		return step.GetJoin().GetThreads()

	case step.GetCancel() != nil:
		return step.GetCancel().GetThreads()
	}

	return nil
}

// _Forks checks that a fork carrying its call carries the call its thread runs.
//
// A fork with no call is not refused: a graph written before forks carried one
// is still a graph, and its thread's first step says what runs.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Forks(graph *workflowpb.Graph) error {
	runs := make(map[string]*workflowpb.Call)

	for _, thread := range graph.GetThreads() {
		runs[thread.GetId()] = _First(thread)
	}

	for _, thread := range graph.GetThreads() {
		for _, step := range thread.GetStatic().GetSteps() {
			fork := step.GetFork()
			if fork.GetFunc() == nil {
				continue
			}

			if !proto.Equal(fork.GetFunc(), runs[fork.GetThread()]) {
				return fmt.Errorf("%s: %w", fork.GetThread(), ErrForkCall)
			}
		}
	}

	return nil
}

// _Spines checks that a graph carries authored threads, that each says what it
// runs, and that only one of them is the spine.
//
// A thread says what it runs in its first step, so a thread with no steps
// names nothing and nothing can be generated for it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: reads the first step rather than an entry field, and
//     the spine is the thread whose id says so
func _Spines(graph *workflowpb.Graph) error {
	spines := 0

	for _, thread := range graph.GetThreads() {
		if thread.GetLive() != nil {
			return fmt.Errorf("%s: %w", thread.GetId(), ErrLive)
		}

		if _First(thread) == nil {
			return fmt.Errorf("%s: %w", thread.GetId(), ErrNoEntry)
		}

		if thread.GetId() != SPINE {
			continue
		}

		spines++

		if spines > 1 {
			return fmt.Errorf("%s: %w", thread.GetId(), ErrTwoSpines)
		}
	}

	return nil
}

// _Parentage checks that every thread a fork starts is named as a child of the
// thread that forked it.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Parentage(graph *workflowpb.Graph) error {
	for _, thread := range graph.GetThreads() {
		for _, step := range thread.GetStatic().GetSteps() {
			fork := step.GetFork()
			if fork == nil {
				continue
			}

			if !_Descends(fork.GetThread(), thread.GetId()) {
				return fmt.Errorf("%s under %s: %w",
					fork.GetThread(), thread.GetId(), ErrParentage)
			}
		}
	}

	return nil
}

// _Descends reports whether an id names this parent.
//
// The spine is the special case it is everywhere else: it contributes no
// prefix, so its children are thread_1 rather than thread_0_1.
//
// Nothing descends from itself, said once here rather than inside each arm.
// The general arm already excluded it by construction - thread_1 does not
// begin with "thread_1_" - but the spine's arm did not, so a fork of thread_0
// declared on thread_0 read as an ordinary child and Check passed a graph
// whose spine forks itself.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-24 16:38: nothing is its own child, which the spine's arm let
//     through
//   - 2026-09-24 16:44: an id with no ordinal is nobody's child. thread_ is
//     not thread_0, so the self-check does not reach it, and an empty ordinal
//     holds no underscore - so it read as an ordinary child of the spine
//   - 2026-09-24 17:12: an ordinal is a decimal counting from one, on both
//     arms, so an id the scheduler would never mint is refused rather than
//     read as an ordinary child
func _Descends(id string, parent string) bool {
	if id == parent {
		return false
	}

	if parent == SPINE {
		return strings.HasPrefix(id, THREAD) && _Decimal(_Ordinal(id))
	}

	return strings.HasPrefix(id, parent+"_") && _Decimal(id[len(parent)+1:])
}

// _Decimal reports whether a segment is an ordinal an id may carry.
//
// A decimal counting from one, with no leading zero: 1, 2, 10. The scheduler
// numbers lanes that way and mints nothing else, so an id carrying anything
// else names a thread this runtime would never have produced - and Check
// exists for graphs a user interface sends, which is where such an id comes
// from.
//
// Empty is refused, and so is anything beginning with zero. That includes the
// spine's own ordinal, so this covers the self-fork a second time; the guard
// above still states that rule on its own, because it is a rule about
// parentage rather than about how an ordinal is spelled, and a later change
// to what an ordinal may look like must not quietly reopen it.
//
// Revisions:
//   - 2026-09-24 17:12: initial creation
func _Decimal(text string) bool {
	if text == "" || text[0] == '0' {
		return false
	}

	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return false
		}
	}

	return true
}

// _Ordinal is what follows a thread id's prefix.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Ordinal(id string) string {
	return strings.TrimPrefix(id, THREAD)
}

// _Arity checks that no call passes more values than the function it names can
// take.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: the thread's own call is among its steps
func _Arity(graph *workflowpb.Graph) error {
	takes := make(map[string]int)

	for _, fn := range graph.GetFunctions() {
		takes[fn.GetName()] = len(fn.GetParams())
	}

	for _, thread := range graph.GetThreads() {
		// Every step, the first included: what a thread runs is a call like
		// any other, so nothing has to be checked twice.
		for _, step := range thread.GetStatic().GetSteps() {
			err := _Fits(takes, _Invoked(step))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// _Fits checks one call against the parameters of the function it names.
//
// A call naming a function the graph does not declare is left alone: that is
// ErrNoBody's business at generation, and two messages for one fault help
// nobody.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Fits(takes map[string]int, call *workflowpb.Call) error {
	if call == nil {
		return nil
	}

	params, known := takes[call.GetFunction()]
	if !known || len(call.GetArgs()) <= params {
		return nil
	}

	return fmt.Errorf("%s takes %d and is passed %d: %w",
		call.GetFunction(), params, len(call.GetArgs()), ErrArity)
}

// _Invoked is the call a step makes, or nil for one that makes none.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Invoked(step *workflowpb.Step) *workflowpb.Call {
	switch {
	case step.GetCall() != nil:
		return step.GetCall()

	case step.GetRepeat() != nil:
		return step.GetRepeat().GetCall()

	case step.GetRetry() != nil:
		return step.GetRetry().GetCall()

	case step.GetTimeout() != nil:
		return step.GetTimeout().GetCall()
	}

	return nil
}
