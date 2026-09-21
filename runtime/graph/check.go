package graph

import (
	"errors"
	"fmt"
	"strings"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

var (
	// ErrLive is returned for a graph whose thread carries the running half. A
	// Graph describes what will happen; progress belongs to a Workflow.
	ErrLive = errors.New("a graph carries no progress")

	// ErrParentage is returned for a thread id that does not descend from the
	// thread that forks it.
	//
	// This is .doc/workflow.md §9's worry in the shape a hierarchical id gives
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

	return _Arity(graph)
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
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Descends(id string, parent string) bool {
	if parent == SPINE {
		return strings.HasPrefix(id, THREAD) && !strings.Contains(_Ordinal(id), "_")
	}

	return strings.HasPrefix(id, parent+"_") && !strings.Contains(id[len(parent)+1:], "_")
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
