package graph

import (
	"errors"
	"fmt"
	"strings"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ErrNoEntry is returned for a thread with no steps. A thread says what it
	// runs in its first step, so one with none names no function: nothing can
	// be generated for it and a spawn of it says nothing.
	ErrNoEntry = errors.New("thread names no function")

	// ErrNoThread is returned for a builtin naming a thread the graph does not
	// declare. This is the refusal a UI reaches by building a graph wrong
	// rather than by writing a bad body, so it names the thread.
	ErrNoThread = errors.New("no such thread")

	// ErrNoBody is returned for a function neither run by a thread nor
	// carrying a body: a name with nothing behind it.
	ErrNoBody = errors.New("function has no body")

	// ErrNoValue is returned for an argument kind structpb does not have.
	// Nothing produces one; it exists so the switch has no silent default.
	ErrNoValue = errors.New("argument has no value")
)

const (
	// The vocabulary a script and a graph share, from the one package that
	// holds it. These were declared here for three increments, with a comment
	// saying they were repeated rather than imported because generating a
	// script should not depend on compiling one. That reason was right; the
	// copy was not, and spelling is the reason without the copy.
	HANDLE = spelling.HANDLE
	THREAD = spelling.THREAD
	SPINE  = spelling.SPINE
	SPAWN  = spelling.SPAWN
	JOIN   = spelling.JOIN
	CANCEL = spelling.CANCEL
	ENTRY  = spelling.ENTRY

	// LAMBDA is how a site that passes arguments is wrapped. Only a generated
	// script says this, so it is not shared.
	LAMBDA = "lambda: "

	// FIRST_STEP is where a thread says what it runs, and BODY_FROM where the
	// steps of that function begin.
	FIRST_STEP = 1
	BODY_FROM  = 1

	// TRUE and FALSE are how Starlark spells a boolean, and NONE its absence.
	TRUE  = "True"
	FALSE = "False"
	NONE  = "None"

	// WHOLE is the format a number takes when it has no fractional part, and
	// FRACTION when it has. A JSON number is one type where Starlark has two,
	// and the two differ under // and %.
	WHOLE    = 'f'
	FRACTION = -1
)

// Runs reports whether a thread runs this function and says anything about it,
// which is what makes its body generated rather than authored.
//
// The second half is the whole of it. A thread that names a function and holds
// no steps says nothing more about it - that is how a spawn points at a leaf.
// Treating that as "generated" drops the leaf's authored body and emits an
// empty function, which is the concurrent graph above rendering
// first and second as pass.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-20 21:03: requires steps past the entry call, so a thread that
//     only names a leaf leaves that leaf's body alone
//   - 2026-09-21 00:59: the entry is a field rather than the first step, so any
//     step at all is a body this generates
//   - 2026-09-21 23:53: reversed: the first step is what the thread runs, so a
//     body is what follows it
func (g *_Gen) Runs(fn *workflowpb.Function) bool {
	thread := g._Thread(fn.GetName())

	return thread != nil && len(thread.GetStatic().GetSteps()) > FIRST_STEP
}

// Steps is the body generated from the thread that runs this function,
// indented one level.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-21 00:59: generates from every step, since the entry is no longer
//     one of them
//   - 2026-09-21 23:53: reversed: generates from the steps past the first,
//     which is the thread's own call
func (g *_Gen) Steps(fn *workflowpb.Function) (string, error) {
	thread := g._Thread(fn.GetName())
	if thread == nil {
		return "", fmt.Errorf("%s: %w", fn.GetName(), ErrNoEntry)
	}

	var lines []string

	for _, step := range thread.GetStatic().GetSteps()[BODY_FROM:] {
		written, err := g._Lines(step)
		if err != nil {
			return "", fmt.Errorf("%s: %w", fn.GetName(), err)
		}

		for _, line := range written {
			lines = append(lines, INDENT+line)
		}
	}

	return strings.Join(lines, "\n"), nil
}

// _Thread is the thread that runs this function, or nil.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-21 00:59: matches a thread's entry rather than its first step
//   - 2026-09-21 23:53: reversed: matches its first step again
func (g *_Gen) _Thread(name string) *workflowpb.Thread {
	for _, thread := range g.graph.GetThreads() {
		if _Runs(thread) == name {
			return thread
		}
	}

	return nil
}

// _Runs is the name of the function a thread runs, which is what its first
// step calls.
//
// The spine is no special case: it carries the entry point as its first step
// like any other thread carries what it was spawned with.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
//   - 2026-09-21 23:53: reads the first step rather than a field
func _Runs(thread *workflowpb.Thread) string {
	return _First(thread).GetFunction()
}

// _First is the call a thread's first step makes, or nil when it has none.
//
// Revisions:
//   - 2026-09-21 23:53: initial creation
func _First(thread *workflowpb.Thread) *workflowpb.Call {
	steps := thread.GetStatic().GetSteps()
	if len(steps) == 0 {
		return nil
	}

	return steps[0].GetCall()
}

// _Lines is one step as the statements a script would write, relative to the
// body they sit in.
//
// Several lines rather than one, because a branch is a block. They carry their
// own indentation relative to each other and none of their own absolutely,
// which is what lets Steps put the body's level in front of every one.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation, as _Line
//   - 2026-09-20 21:05: returns lines, so a branch can be a block
//   - 2026-09-21 01:32: a Cancel arm, and thread ids are strings
func (g *_Gen) _Lines(step *workflowpb.Step) ([]string, error) {
	switch {
	case step.GetCall() != nil:
		return _One(g._Invocation(step.GetCall()))

	case step.GetFork() != nil:
		return _One(g._Fork(step.GetFork()))

	case step.GetJoin() != nil:
		return _One(g._Waits(JOIN, step.GetJoin().GetThreads()))

	case step.GetCancel() != nil:
		return _One(g._Waits(CANCEL, step.GetCancel().GetThreads()))

	case step.GetSleep() != nil:
		return []string{_Pause(step.GetSleep())}, nil

	case step.GetRepeat() != nil, step.GetRetry() != nil, step.GetTimeout() != nil:
		return _One(g._Bounded(step))

	case step.GetIf() != nil:
		return g._If(step.GetIf())

	case step.GetMatch() != nil:
		return g._Match(step.GetMatch())
	}

	return nil, nil
}

// _One is a single-line step as the lines the caller wants.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func _One(line string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}

	return []string{line}, nil
}
