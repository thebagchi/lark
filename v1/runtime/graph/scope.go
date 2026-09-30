package graph

import (
	"fmt"
	"sort"
	"strings"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// ORDINAL_MARK is what comes before each ordinal in a thread id after the
// first, as spelling.Child writes it: thread_1_2 is thread_1, the mark, then 2.
const ORDINAL_MARK = "_"

// _Scope is what a parameter may name, read once from a graph: every name the
// graph declares at its top level, each function's params, and the function
// each thread runs.
//
// A parameter is written into generated source as the name it holds, so the
// one question worth asking is the one Starlark will ask when the script
// compiles: can the call see it. The calling function's params first, then the
// file's top level.
type _Scope struct {
	globals map[string]bool
	params  map[string]map[string]bool
	runs    map[string]string
}

// _ScopeOf reads the scope of every call in a graph.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _ScopeOf(graph *workflowpb.Graph) *_Scope {
	scope := &_Scope{
		globals: make(map[string]bool),
		params:  make(map[string]map[string]bool),
		runs:    make(map[string]string),
	}

	for _, fn := range graph.GetFunctions() {
		scope.globals[fn.GetName()] = true
		scope.params[fn.GetName()] = make(map[string]bool)

		for _, name := range fn.GetParams() {
			scope.params[fn.GetName()][name] = true
		}
	}

	for name := range graph.GetConstants() {
		scope.globals[name] = true
	}

	for name := range graph.GetArgs() {
		scope.globals[name] = true
	}

	for _, thread := range graph.GetThreads() {
		scope.runs[thread.GetId()] = _First(thread).GetFunction()
	}

	return scope
}

// _Resolved checks that every parameter in the graph names something its call
// can see, and that no constant passes one.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func (s *_Scope) _Resolved(graph *workflowpb.Graph) error {
	for _, thread := range graph.GetThreads() {
		err := s._Steps(thread)
		if err != nil {
			return err
		}
	}

	return _Computed(graph)
}

// _Steps checks the parameters of every call one thread's steps make.
//
// A thread's first step is the call its parent made, so its names are the
// parent's to see: spawn(lambda: fetch(url)) inside task(url) passes task's
// url. Every later step is the thread's own function calling.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func (s *_Scope) _Steps(thread *workflowpb.Thread) error {
	for idx, step := range thread.GetStatic().GetSteps() {
		caller := s.runs[thread.GetId()]
		if idx < BODY_FROM {
			caller = s.runs[_Parent(thread.GetId())]
		}

		for _, call := range _Calls(step) {
			err := s._Sees(caller, call)
			if err != nil {
				return fmt.Errorf("%s: %w", thread.GetId(), err)
			}
		}
	}

	return nil
}

// _Sees checks one call's parameters against what caller can see.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func (s *_Scope) _Sees(caller string, call *workflowpb.Call) error {
	for _, arg := range call.GetArgs() {
		if arg.GetParam() == nil {
			return fmt.Errorf("%s: %w", call.GetFunction(), ErrNoValue)
		}

		named, ok := arg.GetParam().(*workflowpb.Parameters_Parameter)
		if !ok {
			continue
		}

		seen := s.params[caller][named.Parameter] || s.globals[named.Parameter]
		if !seen {
			return fmt.Errorf("%s passes %s: %w",
				call.GetFunction(), named.Parameter, ErrUnresolved)
		}
	}

	return nil
}

// _Calls is every call a step makes: the step's own, a fork's, a wrapper's,
// and each a branch or a match could take.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Calls(step *workflowpb.Step) []*workflowpb.Call {
	calls := []*workflowpb.Call{
		step.GetCall(),
		step.GetFork().GetFunc(),
		step.GetRepeat().GetCall(),
		step.GetRetry().GetCall(),
		step.GetTimeout().GetCall(),
		step.GetIf().GetCondition().GetCall(),
		step.GetIf().GetThen(),
		step.GetIf().GetElse(),
		step.GetMatch().GetExpression().GetCall(),
		step.GetMatch().GetDefault(),
	}

	for _, arm := range step.GetMatch().GetCases() {
		calls = append(calls, arm.GetCall())
	}

	made := make([]*workflowpb.Call, 0, len(calls))

	for _, call := range calls {
		if call != nil {
			made = append(made, call)
		}
	}

	return made
}

// _Computed checks that a constant's call passes values only.
//
// A generated file binds its constants at the top, before anything a parameter
// could name is in reach. Constants are read in sorted order, so the refusal a
// graph earns is the same on every check.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Computed(graph *workflowpb.Graph) error {
	names := make([]string, 0, len(graph.GetConstants()))

	for name := range graph.GetConstants() {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		for _, arg := range graph.GetConstants()[name].GetCall().GetArgs() {
			_, named := arg.GetParam().(*workflowpb.Parameters_Parameter)
			if named {
				return fmt.Errorf("constant %s passes %s: %w",
					name, arg.GetParameter(), ErrUnresolved)
			}
		}
	}

	return nil
}

// _Parent is the id of the thread that forked id, or empty for the spine.
//
// An id names its parent: thread_1_2 is thread_1's second child. The spine is
// the one parent that contributes no prefix, so thread_1's parent is the spine.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
func _Parent(id string) string {
	if id == SPINE {
		return ""
	}

	cut := strings.LastIndex(id, ORDINAL_MARK)
	if cut < len(THREAD) {
		return SPINE
	}

	return id[:cut]
}
