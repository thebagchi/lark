package graph

import (
	"fmt"
	"strings"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Both directions of one subject live here: reading a script's threads out of
// its source, and writing a graph's threads back as the lines a script would
// have. A reader asking how a spawn works finds both in one file, as they do
// for the wrappers in bounded.go and the branches in branch.go.

// _Lane is one thread's own state while its function is being read.
//
// The ordinal counts children of this thread rather than threads in the run,
// which is what makes an id a fact about structure: one counter shared by the
// derivation would number in the order spawns are met, and a sibling met after
// a nephew would take the higher number.
type _Lane struct {
	id      string
	ordinal int
	bound   map[string]string
}

// _Thread reads one function as a thread of its own, and every function it
// spawns as another.
//
// The thread is appended before its body is read, so a parent precedes its
// children however deeply they nest.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Thread(id string, entry *workflowpb.Call, def *syntax.DefStmt) {
	if r._Reading(def.Name.Name) {
		r._Gave(def.Name.Name, "calls itself through a spawn")

		return
	}

	r.chain = append(r.chain, def.Name.Name)
	defer func() { r.chain = r.chain[:len(r.chain)-1] }()

	thread := &workflowpb.Thread{Id: id, Entry: entry}
	r.threads = append(r.threads, thread)

	lane := &_Lane{id: id, bound: make(map[string]string)}

	var steps []*workflowpb.Step

	whole := true

	for idx := 0; idx < len(def.Body); idx++ {
		// A match is two statements and is read as one step. Reading them apart
		// would be two steps for one decision, and would re-emit as a program
		// that calls its expression once per case.
		step := r._Matched(def.Body, idx)
		if step != nil {
			steps = append(steps, step)
			idx++

			continue
		}

		made, ok := r._Statement(lane, def.Body[idx])

		steps = append(steps, made...)
		whole = whole && ok
	}

	// Steps or a body, never both. A thread that did not model its function
	// keeps its entry and drops its steps, and the emitter's rule - a thread
	// generates a body only when it has steps - then carries the authored text
	// instead. Threads its body spawned are kept either way: they exist.
	if !whole {
		steps = nil
	}

	thread.State = &workflowpb.Thread_Static{Static: &workflowpb.Static{Steps: steps}}
}

// _Reading reports whether this function is already being read, which is a
// cycle.
//
// The chain, not a set of everything seen. A set answers "seen it" for the
// second of four identical spawns, which is how samples/state.star loses three
// of its seven threads while the graph still calls itself complete.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Reading(name string) bool {
	for _, held := range r.chain {
		if held == name {
			return true
		}
	}

	return false
}

// _Spawn is the step a spawn is, having read what it starts as a thread.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Spawn(lane *_Lane, call *syntax.CallExpr) ([]*workflowpb.Step, bool) {
	if len(call.Args) != 1 {
		r._Gave(SPAWN, "takes one function")

		return nil, false
	}

	entry, def := r._Target(call.Args[0])
	if def == nil {
		r._Gave(SPAWN, "names nothing this file defines")

		return nil, false
	}

	lane.ordinal++

	id := _Child(lane.id, lane.ordinal)

	r._Thread(id, entry, def)

	return []*workflowpb.Step{{
		Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: id}},
	}}, true
}

// _Target is the call a spawned site makes, and the function it runs.
//
// Two spellings. A bare name is the function itself; a lambda whose body is a
// single call is that call, which is how a spawned site carries arguments -
// spawn passes none on, so a generator renders a fork with arguments as
// spawn(lambda: greet("alice")). Refusing that would make the round trip
// impossible on a script this project generates itself.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Target(expr syntax.Expr) (*workflowpb.Call, *syntax.DefStmt) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		def := r.defs[actual.Name]
		if def == nil {
			return nil, nil
		}

		return &workflowpb.Call{Function: actual.Name}, def

	case *syntax.LambdaExpr:
		inner, ok := actual.Body.(*syntax.CallExpr)
		if !ok {
			return nil, nil
		}

		name, ok := inner.Fn.(*syntax.Ident)
		if !ok {
			return nil, nil
		}

		def := r.defs[name.Name]
		if def == nil {
			return nil, nil
		}

		return &workflowpb.Call{Function: name.Name, Args: _Values(inner)}, def
	}

	return nil, nil
}

// _Waits is the step a join or a cancel is, and the spawns its arguments made.
//
// A handle is usually a variable rather than a spawn written inside the join,
// so the arguments are resolved through what this thread has bound as well as
// read directly. An argument it can resolve neither way contributes no thread:
// a join that names fewer threads says less, where one that names the wrong
// thread lies.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Waits(
	lane *_Lane,
	call *syntax.CallExpr,
	name string,
) ([]*workflowpb.Step, bool) {
	var steps []*workflowpb.Step

	var ids []string

	for _, arg := range call.Args {
		held, ok := arg.(*syntax.Ident)
		if ok && lane.bound[held.Name] != "" {
			ids = append(ids, lane.bound[held.Name])

			continue
		}

		made, _ := r._Expression(lane, arg)
		steps = append(steps, made...)

		id, ok := _Started(made)
		if ok {
			ids = append(ids, id)
		}
	}

	return append(steps, _Waited(name, ids)), len(ids) == len(call.Args)
}

// _Bind remembers which thread a name holds the handle of, and reports
// whether it did.
//
// Only a plain assignment of a spawn. A name bound any other way, rebound, or
// assigned inside a branch is beyond this, and beyond it means a later join
// names fewer threads rather than the wrong ones.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Bind(lane *_Lane, stmt *syntax.AssignStmt, steps []*workflowpb.Step) bool {
	if stmt.Op != syntax.EQ {
		return false
	}

	name, ok := stmt.LHS.(*syntax.Ident)
	if !ok {
		return false
	}

	id, ok := _Started(steps)
	if !ok {
		return false
	}

	lane.bound[name.Name] = id

	return true
}

// _Gave records a giving-up, which travels back beside the graph.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func (r *_Reading) _Gave(what string, why string) {
	r.unknown = append(r.unknown, fmt.Sprintf("%s: %s", what, why))
}

// _Child is the id of a thread's nth child.
//
// The spine is the one special case: it contributes no prefix, so its children
// are thread_1 and thread_2 rather than thread_0_1. Every thread descends from
// it, and a prefix every id carries says nothing.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Child(parent string, ordinal int) string {
	if parent == SPINE {
		return fmt.Sprintf("%s%d", THREAD, ordinal)
	}

	return fmt.Sprintf("%s_%d", parent, ordinal)
}

// _Waited is the step a join or a cancel is, naming these threads.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation, as _Names
//   - 2026-09-21 01:32: builds the message the step kind has of its own, so the
//     thread list is a list rather than values to be read back as strings
func _Waited(name string, ids []string) *workflowpb.Step {
	if name == CANCEL {
		return &workflowpb.Step{
			Action: &workflowpb.Step_Cancel{Cancel: &workflowpb.Cancel{Threads: ids}},
		}
	}

	return &workflowpb.Step{
		Action: &workflowpb.Step_Join{Join: &workflowpb.Join{Threads: ids}},
	}
}

// _Started is the thread the last of these steps spawned, if it spawned one.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
func _Started(steps []*workflowpb.Step) (string, bool) {
	if len(steps) == 0 {
		return "", false
	}

	fork := steps[len(steps)-1].GetFork()
	if fork == nil {
		return "", false
	}

	return fork.GetThread(), true
}

// _Matched is the Match the statements at this position state together, or
// nil.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Matched(body []syntax.Stmt, idx int) *workflowpb.Step {
	if idx+1 >= len(body) {
		return nil
	}

	assign, ok := body[idx].(*syntax.AssignStmt)
	if !ok {
		return nil
	}

	chain, ok := body[idx+1].(*syntax.IfStmt)
	if !ok {
		return nil
	}

	return r._Match(assign, chain)
}

// _Fork is a spawn, bound to the handle its thread's id names.
//
// A fork names a thread, not a function. What runs there is that thread's own
// entry, which is also where a site's arguments live - so rendering one is two
// lookups and the second is the one that matters.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (g *_Gen) _Fork(fork *workflowpb.Fork) (string, error) {
	thread, err := g._Named(fork.GetThread())
	if err != nil {
		return "", err
	}

	site, err := g._Site(thread.GetEntry())
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s = %s(%s)", _Handle(fork.GetThread()), SPAWN, site), nil
}

// _Waits is a join or a cancel, naming the handles of the threads it names, in
// the order it names them.
//
// One function for both, because the two differ only in the word they write.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (g *_Gen) _Waits(name string, threads []string) (string, error) {
	var handles []string

	for _, id := range threads {
		_, err := g._Named(id)
		if err != nil {
			return "", err
		}

		handles = append(handles, _Handle(id))
	}

	return fmt.Sprintf("%s(%s)", name, strings.Join(handles, SEPARATOR)), nil
}

// _Named is the thread this id declares, or why there is none.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (g *_Gen) _Named(id string) (*workflowpb.Thread, error) {
	for _, thread := range g.graph.GetThreads() {
		if thread.GetId() != id {
			continue
		}

		if thread.GetEntry() == nil {
			return nil, fmt.Errorf("%s: %w", id, ErrNoEntry)
		}

		return thread, nil
	}

	return nil, fmt.Errorf("%s: %w", id, ErrNoThread)
}

// _Handle is the name a spawned thread's handle takes.
//
// The thread's id with its prefix swapped, so thread_1 is h1 and thread_1_1 is
// h1_1. Named after the thread rather than the function because
// first = spawn(first) shadows what it just spawned, and two spawns of one
// function would reuse the name. The id is unique, so the handle is.
//
// Revisions:
//   - 2026-09-20 21:02: initial creation
//   - 2026-09-21 01:32: named after the thread id, which is a string that names
//     its parent, rather than after a list slot
func _Handle(id string) string {
	return HANDLE + strings.TrimPrefix(id, THREAD)
}
