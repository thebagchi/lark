package graph

import (
	"errors"
	"fmt"
	"strings"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

var (
	// ErrNoEntry is returned for a thread whose first step is not a Call. Such
	// a thread names no function, so nothing can be generated for it and a fork
	// to it says nothing.
	ErrNoEntry = errors.New("thread names no function")

	// ErrNoSlot is returned for a Fork or a Join naming a slot outside
	// Graph.threads. This is the refusal a UI reaches by building a graph
	// wrong rather than by writing a bad body, so it names the slot.
	ErrNoSlot = errors.New("no such thread")

	// ErrNoBody is returned for a function neither run by a thread nor
	// carrying a body: a name with nothing behind it.
	ErrNoBody = errors.New("function has no body")

	// ErrNoValue is returned for an argument kind structpb does not have.
	// Nothing produces one; it exists so the switch has no silent default.
	ErrNoValue = errors.New("argument has no value")
)

const (
	// HANDLE prefixes a spawned thread's handle, which is numbered by slot.
	HANDLE = "h"

	// SPAWN, JOIN and LAMBDA are what a generated body calls and how it wraps
	// a site that passes arguments.
	SPAWN  = "spawn"
	JOIN   = "join"
	LAMBDA = "lambda: "

	// FIRST is the step naming a thread's own function, which is not part of
	// the body it generates.
	FIRST = 1

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

// Runs reports whether some thread's first Call names this function **and has
// steps past it**, which is what makes its body generated rather than authored.
//
// The second half is the whole of it. A thread holding only its own Call names
// a function and says nothing more about it - that is how a fork points at a
// leaf. Treating that as "generated" drops the leaf's authored body and emits
// an empty function, which is the concurrent graph in .doc/workflow.md
// rendering first and second as pass.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-20 21:03: requires steps past the entry call, so a thread that
//     only names a leaf leaves that leaf's body alone
func (g *_Gen) Runs(fn *workflowpb.Function) bool {
	thread := g._Thread(fn.GetName())

	return thread != nil && len(thread.GetSteps()) > FIRST
}

// Steps is the body generated from the thread that runs this function,
// indented one level.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) Steps(fn *workflowpb.Function) (string, error) {
	thread := g._Thread(fn.GetName())
	if thread == nil {
		return "", fmt.Errorf("%s: %w", fn.GetName(), ErrNoEntry)
	}

	var lines []string

	for _, step := range thread.GetSteps()[FIRST:] {
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

// _Thread is the thread whose first step calls this function, or nil.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Thread(name string) *workflowpb.GraphThread {
	for _, thread := range g.graph.GetThreads() {
		steps := thread.GetSteps()
		if len(steps) == 0 {
			continue
		}

		if steps[0].GetCall().GetFunction() == name {
			return thread
		}
	}

	return nil
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
func (g *_Gen) _Lines(step *workflowpb.Step) ([]string, error) {
	switch {
	case step.GetCall() != nil:
		return _One(g._Invocation(step.GetCall()))

	case step.GetFork() != nil:
		return _One(g._Fork(step.GetFork()))

	case step.GetJoin() != nil:
		return _One(g._Join(step.GetJoin()))

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

// _Fork is a spawn, bound to the handle its slot is numbered by.
//
// A fork names a slot, not a function. What runs there is that slot's own first
// Call, which is also where a site's arguments live - so rendering one is two
// lookups and the second is the one that matters.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Fork(fork *workflowpb.Fork) (string, error) {
	slot := fork.GetThread()

	thread, err := g._Slot(slot)
	if err != nil {
		return "", err
	}

	site, err := g._Site(thread.GetSteps()[0].GetCall())
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s = %s(%s)", _Handle(slot), SPAWN, site), nil
}

// _Join waits for the handles its slots are numbered by, in the order given.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Join(join *workflowpb.Join) (string, error) {
	var handles []string

	for _, slot := range join.GetThreads() {
		_, err := g._Slot(slot)
		if err != nil {
			return "", err
		}

		handles = append(handles, _Handle(slot))
	}

	return fmt.Sprintf("%s(%s)", JOIN, strings.Join(handles, SEPARATOR)), nil
}

// _Slot is the thread at this position, or why there is none.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func (g *_Gen) _Slot(slot int32) (*workflowpb.GraphThread, error) {
	threads := g.graph.GetThreads()

	if slot < 0 || int(slot) >= len(threads) {
		return nil, fmt.Errorf("%d of %d: %w", slot, len(threads), ErrNoSlot)
	}

	thread := threads[slot]
	if len(thread.GetSteps()) == 0 || thread.GetSteps()[0].GetCall() == nil {
		return nil, fmt.Errorf("%d: %w", slot, ErrNoEntry)
	}

	return thread, nil
}

// _Handle is the name a spawned thread's handle takes.
//
// Numbered by slot rather than after the function: first = spawn(first) shadows
// what it just spawned, and a second fork of one function would reuse the name.
// Numbering is safe without bookkeeping because a body a thread generates is
// generated in full, so nothing authored shares the scope.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
func _Handle(slot int32) string {
	return fmt.Sprintf("%s%d", HANDLE, slot)
}
