package graph

import (
	"errors"
	"fmt"
	"strings"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

var (
	// ErrNoEntry is returned for a spawned thread carrying no entry. Such a
	// thread names no function, so nothing can be generated for it and a spawn
	// of it says nothing. Only the spine may omit one.
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
	// HANDLE prefixes a spawned thread's handle, and THREAD prefixes every
	// thread id. A handle is its thread's id with the one swapped for the
	// other, so thread_1 is h1 and thread_1_1 is h1_1 - unique because the id
	// is, and readable back to the thread it waits for.
	HANDLE = "h"
	THREAD = "thread_"

	// SPINE is the entry point's thread. It must stay equal to scheduler.SPINE,
	// which the same test asserts as ENTRY.
	SPINE = THREAD + "0"

	// SPAWN, JOIN and CANCEL are the builtins that name threads rather than
	// values, and LAMBDA is how a site that passes arguments is wrapped.
	SPAWN  = "spawn"
	JOIN   = "join"
	CANCEL = "cancel"
	LAMBDA = "lambda: "

	// ENTRY is the function the spine runs. A thread with no entry is the
	// spine, and what runs there is the artifact's entry point rather than
	// anything the graph says - so this must stay equal to artifact.ENTRY,
	// which a test asserts. It is repeated rather than imported because
	// generating a script should not depend on compiling one.
	ENTRY = "main"

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
// empty function, which is the concurrent graph in .doc/workflow.md rendering
// first and second as pass.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-20 21:03: requires steps past the entry call, so a thread that
//     only names a leaf leaves that leaf's body alone
//   - 2026-09-21 00:59: the entry is a field rather than the first step, so any
//     step at all is a body this generates
func (g *_Gen) Runs(fn *workflowpb.Function) bool {
	thread := g._Thread(fn.GetName())

	return thread != nil && len(thread.GetStatic().GetSteps()) > 0
}

// Steps is the body generated from the thread that runs this function,
// indented one level.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-21 00:59: generates from every step, since the entry is no longer
//     one of them
func (g *_Gen) Steps(fn *workflowpb.Function) (string, error) {
	thread := g._Thread(fn.GetName())
	if thread == nil {
		return "", fmt.Errorf("%s: %w", fn.GetName(), ErrNoEntry)
	}

	var lines []string

	for _, step := range thread.GetStatic().GetSteps() {
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
func (g *_Gen) _Thread(name string) *workflowpb.Thread {
	for _, thread := range g.graph.GetThreads() {
		if _Runs(thread) == name {
			return thread
		}
	}

	return nil
}

// _Runs is the name of the function a thread runs.
//
// A thread with no entry is the spine, and what runs there is the entry point.
// Saying so here rather than at each caller is what keeps the spine from being
// a special case anywhere else.
//
// Revisions:
//   - 2026-09-21 00:59: initial creation
func _Runs(thread *workflowpb.Thread) string {
	if thread.GetEntry() == nil {
		return ENTRY
	}

	return thread.GetEntry().GetFunction()
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
