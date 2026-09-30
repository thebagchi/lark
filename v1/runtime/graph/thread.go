package graph

import (
	"fmt"
	"strings"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/spelling"
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
//
// names is every name a call on this lane may pass as a parameter: the
// function's own params, and the functions, constants and arguments the file
// declares. A handle is bound, never a parameter - the one thing a script does
// with a thread is join it.
type _Lane struct {
	id      string
	ordinal int
	bound   map[string]string
	names   map[string]bool
}

// _Thread reads one function as a thread of its own, and every function it
// spawns as another.
//
// The thread is appended before its body is read, so a parent precedes its
// children however deeply they nest.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-21 23:53: what the thread runs is its first step, not a field of
//     its own
//   - 2026-09-30 00:41: gives the lane the names its calls may pass
func (r *_Reading) _Thread(id string, entry *workflowpb.Call, def *syntax.DefStmt) {
	if r._Reading(def.Name.Name) {
		r._Gave(def.Name.Name, "calls itself through a spawn")

		return
	}

	r.chain = append(r.chain, def.Name.Name)
	defer func() { r.chain = r.chain[:len(r.chain)-1] }()

	thread := new(workflowpb.Thread)
	thread.Id = id

	r.threads = append(r.threads, thread)

	lane := &_Lane{id: id, bound: make(map[string]string), names: r._Names(def)}

	// The first step is what this thread runs. Everything read below is that
	// function's own body, and follows it.
	steps := []*workflowpb.Step{{Action: &workflowpb.Step_Call{Call: entry}}}

	whole := true

	for idx := 0; idx < len(def.Body); idx++ {
		// A match is two statements and is read as one step. Reading them apart
		// would be two steps for one decision, and would re-emit as a program
		// that calls its expression once per case.
		step := r._Matched(lane, def.Body, idx)
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
	// keeps its first step and drops the rest, and the emitter's rule - a
	// thread generates a body only when it has more than the one - then
	// carries the authored text instead. Threads its body spawned are kept
	// either way: they exist.
	if !whole {
		steps = steps[:FIRST_STEP]
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
// The fork carries the call as well as the thread, so the forking thread reads
// without walking to each thread it starts. A call whose arguments cannot all
// be carried still starts its thread - the thread exists - but says so, and
// the function holding it keeps its text: generating the spawn from what was
// carried would drop an argument the script passed.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-30 00:41: the fork carries its call, and an argument that cannot
//     be carried is reported rather than dropped
func (r *_Reading) _Spawn(lane *_Lane, call *syntax.CallExpr) ([]*workflowpb.Step, bool) {
	if len(call.Args) != 1 {
		r._Gave(SPAWN, "takes one function")

		return nil, false
	}

	entry, def, carried := r._Target(lane, call.Args[0])
	if def == nil {
		r._Gave(SPAWN, "names nothing this file defines")

		return nil, false
	}

	lane.ordinal++

	id := _Child(lane.id, lane.ordinal)

	r._Thread(id, entry, def)

	if !carried {
		r._Gave(entry.GetFunction(), UNCARRIED)
	}

	fork := &workflowpb.Fork{Thread: id, Func: proto.CloneOf(entry)}

	return []*workflowpb.Step{{Action: &workflowpb.Step_Fork{Fork: fork}}}, carried
}

// _Target is the call a spawned site makes, and the function it runs.
//
// Two spellings. A bare name is the function itself; a lambda whose body is a
// single call is that call, which is how a spawned site carries arguments -
// spawn passes none on, so a generator renders a fork with arguments as
// spawn(lambda: greet("alice")). Refusing that would make the round trip
// impossible on a script this project generates itself.
//
// The third result says whether every argument the lambda passes was carried.
// When one was not, the call carries none, and the caller decides what that
// costs.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-30 00:41: carries a parameter as well as a value, and says when
//     it could not carry an argument rather than dropping it
func (r *_Reading) _Target(
	lane *_Lane,
	expr syntax.Expr,
) (*workflowpb.Call, *syntax.DefStmt, bool) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		def := r.defs[actual.Name]
		if def == nil {
			return nil, nil, false
		}

		return &workflowpb.Call{Function: actual.Name}, def, true

	case *syntax.LambdaExpr:
		inner, ok := actual.Body.(*syntax.CallExpr)
		if !ok {
			return nil, nil, false
		}

		name, ok := inner.Fn.(*syntax.Ident)
		if !ok {
			return nil, nil, false
		}

		def := r.defs[name.Name]
		if def == nil {
			return nil, nil, false
		}

		args, carried := _Passes(lane._Carries, inner)

		return &workflowpb.Call{Function: name.Name, Args: args}, def, carried
	}

	return nil, nil, false
}

// _Names is every name a call inside def may pass as a parameter: def's own
// params, then what the file declares at its top level.
//
// Revisions:
//   - 2026-09-30 00:41: initial creation
func (r *_Reading) _Names(def *syntax.DefStmt) map[string]bool {
	names := make(map[string]bool)

	params, _ := _Params(def)

	for _, name := range params {
		names[name] = true
	}

	for name := range r.defs {
		names[name] = true
	}

	for name := range r.constants {
		names[name] = true
	}

	for name := range r.args {
		names[name] = true
	}

	return names
}

// _Carries reports whether a call on this lane may pass name as a parameter.
//
// A name this lane bound to a handle is out, whatever else it shadows: a
// thread is joined, never passed.
//
// Revisions:
//   - 2026-09-30 00:41: initial creation
func (l *_Lane) _Carries(name string) bool {
	return l.names[name] && l.bound[name] == ""
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
	return spelling.Child(parent, ordinal)
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
//   - 2026-09-30 00:41: reads on a lane, so a matched call may pass a
//     parameter
func (r *_Reading) _Matched(lane *_Lane, body []syntax.Stmt, idx int) *workflowpb.Step {
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

	return r._Match(lane, assign, chain)
}

// _Fork is a spawn, bound to the handle its thread's id names.
//
// What runs there is the fork's own call when it carries one, and its thread's
// first step when it does not - a graph written before a fork carried its call
// is still one this generates. Check has already refused a graph where the two
// disagree, so either is the call.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-30 00:41: generates from the fork's own call when it has one
func (g *_Gen) _Fork(fork *workflowpb.Fork) (string, error) {
	thread, err := g._Named(fork.GetThread())
	if err != nil {
		return "", err
	}

	call := fork.GetFunc()
	if call == nil {
		call = _First(thread)
	}

	site, err := g._Site(call)
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

		if _First(thread) == nil {
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
