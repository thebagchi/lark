package graph

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ERR_ARITY is returned for a Call passing a number of arguments its
	// function does not take, too few or too many.
	//
	// The arguments line up with the parameters index for index, so a count
	// that does not match is a flow that will fail when it runs. Caught here
	// rather than reaching Starlark as an error against a line in generated
	// source, which names nothing a reader can act on. spelling's, so it is the
	// value a compile refuses the same mistake in a script with.
	ERR_ARITY = spelling.ERR_ARITY

	// ERR_ARGUMENTS is returned for a Call carrying both args and operands:
	// two lists for one argument list, with nothing to say which is meant.
	ERR_ARGUMENTS = errors.New("a call carries both args and operands")

	// ERR_NOT_FORKED is returned for a join or a cancel naming a binding that
	// no spawn earlier on its statement list holds, and for two spawns given
	// one binding on one list.
	//
	// A generated script names a thread by the binding its spawn wrote, so a
	// join anywhere else - in the other branch of an if, or before the spawn -
	// names a variable that does not exist, and a second spawn of one name
	// hides the first.
	ERR_NOT_FORKED = errors.New("a thread is joined or cancelled where it was not spawned")

	// ERR_UNRESOLVED is returned for a name that resolves to nothing: not a
	// parameter of the function holding it, a result or a binding earlier on
	// its list, or a function, constant or argument the flow declares. A
	// constant sees only functions and constants, since it is computed before
	// anything else a name could hold is bound.
	ERR_UNRESOLVED = errors.New("a name resolves to nothing")
)

// Check is what a flow must satisfy before anything generates from it.
//
// Exported because a host taking a flow from a user interface needs it, and
// that host is not this package. Emit does not call it: generating is the
// caller's decision and this is the caller's check, so a host that has already
// validated does not pay twice.
//
// _Distinct is part of it, so a host validating a whole flow asks once.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 16:25: includes Distinct, which its own first line had always
//     claimed
//   - 2026-09-30 00:44: refuses a join or a cancel of a thread not forked
//     there, a fork whose call is not its thread's, and a parameter its call
//     cannot see
//   - 2026-10-02 00:13: checks a Flow: bodies, the arity of every call, a
//     call carrying both lists, constants, and each statement list's names
func Check(flow *workflowpb.Flow) error {
	err := _Distinct(flow)
	if err != nil {
		return err
	}

	err = _Bodies(flow)
	if err != nil {
		return err
	}

	err = _Arity(flow)
	if err != nil {
		return err
	}

	err = _Computed(flow)
	if err != nil {
		return err
	}

	return _Resolved(flow)
}

// _Bodies checks that every function, and main, has something to write.
//
// Code unset, an empty statement list, and text that is empty or only comments
// are each nothing: the closer a generator writes is not a body.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Bodies(flow *workflowpb.Flow) error {
	for _, fn := range flow.GetFunctions() {
		var empty bool

		switch held := fn.GetCode().(type) {
		case *workflowpb.Function_Statements:
			empty = len(held.Statements.GetStatement()) == 0

		case *workflowpb.Function_Body:
			empty = _NotesOnly(held.Body)

		default:
			empty = true
		}

		if empty {
			return fmt.Errorf("%s: %w", fn.GetName(), ERR_NO_BODY)
		}
	}

	var empty bool

	switch held := flow.GetSpine().(type) {
	case *workflowpb.Flow_Main:
		empty = len(held.Main.GetStatement()) == 0

	case *workflowpb.Flow_Text:
		empty = _NotesOnly(held.Text)

	default:
		empty = true
	}

	if empty {
		return fmt.Errorf("%s: %w", ENTRY, ERR_NO_BODY)
	}

	return nil
}

// _Arity checks every call the flow makes against the function it names: in
// a statement, a condition, a match, and a constant.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: the thread's own call is among its steps
//   - 2026-10-02 00:13: every call in a Flow, refusing too few as well as too
//     many, a function the flow does not declare, and a call carrying both
//     lists
func _Arity(flow *workflowpb.Flow) error {
	takes := make(map[string]int)

	for _, fn := range flow.GetFunctions() {
		takes[fn.GetName()] = len(fn.GetParams())
	}

	for _, call := range _Calls(flow) {
		err := _Fits(takes, call)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Fits checks one call against the parameters of the function it names.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 00:13: refuses too few, a function the flow does not
//     declare, and both lists at once
func _Fits(takes map[string]int, call *workflowpb.Call) error {
	if _Both(call) {
		return fmt.Errorf("%s: %w", call.GetFunction(), ERR_ARGUMENTS)
	}

	params, known := takes[call.GetFunction()]
	if !known {
		return fmt.Errorf("call of %s: %w", call.GetFunction(), ERR_UNRESOLVED)
	}

	passed := len(call.GetArgs()) + len(call.GetOperands())
	if passed == params {
		return nil
	}

	return fmt.Errorf("%s takes %d and is passed %d: %w",
		call.GetFunction(), params, passed, ERR_ARITY)
}

// _Calls is every call a flow makes, constants first, in sorted order, so the
// refusal a flow earns is the same on every check.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Calls(flow *workflowpb.Flow) []*workflowpb.Call {
	var calls []*workflowpb.Call

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if call := flow.GetConstants()[name].GetCall(); call != nil {
			calls = append(calls, call)
		}
	}

	visit := func(stmt *workflowpb.Statement) {
		calls = append(calls, _Made(stmt)...)
	}

	for _, fn := range flow.GetFunctions() {
		_Each(fn.GetStatements().GetStatement(), visit)
	}

	_Each(flow.GetMain().GetStatement(), visit)

	return calls
}

// _Made is the calls one statement makes itself, not counting those of the
// statements it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Made(stmt *workflowpb.Statement) []*workflowpb.Call {
	made := []*workflowpb.Call{
		stmt.GetCall(),
		stmt.GetSpawn().GetCall(),
		stmt.GetRepeat().GetCall(),
		stmt.GetRetry().GetCall(),
		stmt.GetTimeout().GetCall(),
		stmt.GetIf().GetCondition().GetCall(),
		stmt.GetMatch().GetExpression().GetCall(),
	}

	var calls []*workflowpb.Call

	for _, call := range made {
		if call != nil {
			calls = append(calls, call)
		}
	}

	return calls
}

// _Each visits every statement of a list and every statement those hold, in
// order.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Each(list []*workflowpb.Statement, visit func(*workflowpb.Statement)) {
	for _, stmt := range list {
		_Visit(stmt, visit)
	}
}

// _Visit visits one statement and the statements it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Visit(stmt *workflowpb.Statement, visit func(*workflowpb.Statement)) {
	if stmt == nil {
		return
	}

	visit(stmt)

	_Visit(stmt.GetIf().GetThen(), visit)
	_Visit(stmt.GetIf().GetElse(), visit)
	_Visit(stmt.GetLoop().GetBody(), visit)

	for _, arm := range stmt.GetMatch().GetCases() {
		_Visit(arm.GetStatement(), visit)
	}

	_Visit(stmt.GetMatch().GetDefault(), visit)
}

// _Computed checks that a constant's call names only functions and constants
// declared in the flow, and that no constants name each other in a cycle.
//
// A generated file binds its constants at the top, before anything else a
// name could hold is in reach, and writes each after the constants it names -
// which a cycle makes impossible.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
//   - 2026-10-02 00:13: a constant may name another constant or a function,
//     and a cycle is ERR_CONSTANT
func _Computed(flow *workflowpb.Flow) error {
	functions := make(map[string]bool)

	for _, fn := range flow.GetFunctions() {
		functions[fn.GetName()] = true
	}

	held := flow.GetConstants()

	for _, name := range slices.Sorted(maps.Keys(held)) {
		for _, op := range held[name].GetCall().GetOperands() {
			named := op.GetName()
			if named == "" || functions[named] {
				continue
			}

			if _, constant := held[named]; !constant {
				return fmt.Errorf("constant %s names %s: %w",
					name, named, ERR_UNRESOLVED)
			}
		}
	}

	_, err := _Order(flow)
	if err != nil {
		return fmt.Errorf("constants name each other: %w", ERR_CONSTANT)
	}

	return nil
}

// _Resolved checks each statement list of every function, and of main: every
// name resolves, and every join and cancel names a spawn the list can see.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Resolved(flow *workflowpb.Flow) error {
	globals := make(map[string]bool)

	for _, fn := range flow.GetFunctions() {
		globals[fn.GetName()] = true
	}

	for name := range flow.GetConstants() {
		globals[name] = true
	}

	for name := range flow.GetArgs() {
		globals[name] = true
	}

	for _, fn := range flow.GetFunctions() {
		seen := _Opening(globals, fn.GetParams())

		err := seen._List(fn.GetStatements().GetStatement())
		if err != nil {
			return fmt.Errorf("%s: %w", fn.GetName(), err)
		}
	}

	err := _Opening(globals, nil)._List(flow.GetMain().GetStatement())
	if err != nil {
		return fmt.Errorf("%s: %w", ENTRY, err)
	}

	return nil
}
