package graph

import (
	"fmt"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// Both directions of one subject live here: reading a call of a listed function
// out of a script, and writing a flow's call back as the call a script would
// make.

// _Call is the call a script makes of a function the flow lists, and whether
// the flow can carry it.
//
// Positional arguments only, each a value or a name the scope sees. A keyword,
// an unpacked *args or **kwargs, or an argument computed from an expression
// is one the schema has no place for, and carrying the rest would invent a
// call the script never made.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-30 00:41: reads on a lane, so a call may pass a parameter
//   - 2026-10-02 00:04: returns the Call alone, taking a name as an operand
//     and refusing a function the scope binds to something else
func (r *_Reading) _Call(call *syntax.CallExpr, scope *_Scope) (*workflowpb.Call, bool) {
	name := _Callee(call)
	if !scope._Callable(name) {
		return nil, false
	}

	args, operands, ok := _Carries(call.Args, scope)
	if !ok {
		return nil, false
	}

	return &workflowpb.Call{Function: name, Args: args, Operands: operands}, true
}

// _Carries is a call's arguments as the flow carries them: every one a value,
// as args, or every one an operand when any of them names something.
//
// A call never carries both, so a list of values keeps the shape a reader
// already has for data, and a literal beside a name is an Operand's literal.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation, as _Values
//   - 2026-09-30 00:41: carries a parameter as well as a value, as _Passes
//   - 2026-10-02 00:04: writes args or operands, never both
func _Carries(
	exprs []syntax.Expr,
	scope *_Scope,
) ([]*structpb.Value, []*workflowpb.Operand, bool) {
	var (
		operands []*workflowpb.Operand
		values   []*structpb.Value
		named    bool
	)

	for _, expr := range exprs {
		operand, ok := _Carried(expr, scope)
		if !ok {
			return nil, nil, false
		}

		operands = append(operands, operand)
		values = append(values, operand.GetLiteral())
		named = named || operand.GetName() != ""
	}

	if named {
		return nil, operands, true
	}

	return values, nil, true
}

// _Carried is one argument as the flow carries it, or false when it cannot be.
//
// A value is tried first: True, False and None are names, and they are values
// rather than names a scope binds.
//
// Revisions:
//   - 2026-09-30 00:41: initial creation
//   - 2026-10-02 00:04: an Operand, a literal or a name the scope sees
func _Carried(expr syntax.Expr, scope *_Scope) (*workflowpb.Operand, bool) {
	value, ok := _Arg(expr)
	if ok {
		return &workflowpb.Operand{
			Source: &workflowpb.Operand_Literal{Literal: value},
		}, true
	}

	name, ok := expr.(*syntax.Ident)
	if !ok || !scope._Sees(name.Name) {
		return nil, false
	}

	return &workflowpb.Operand{Source: &workflowpb.Operand_Name{Name: name.Name}}, true
}

// _Target is the call a builtin taking a function makes, and whether the flow
// can carry it.
//
// Two spellings. A bare name is the function itself; a lambda whose body is a
// single call is that call, which is how a spawn or a wrapper passes
// arguments, since it passes none on. A lambda is only that sugar: the call is
// what the thread or the wrapper runs.
//
// Revisions:
//   - 2026-09-21 01:17: initial creation
//   - 2026-09-30 00:41: carries a parameter as well as a value, and says when
//     it could not carry an argument rather than dropping it
//   - 2026-10-02 00:04: reads in a scope, and returns the call alone
func (r *_Reading) _Target(expr syntax.Expr, scope *_Scope) (*workflowpb.Call, bool) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		if !scope._Callable(actual.Name) {
			return nil, false
		}

		return &workflowpb.Call{Function: actual.Name}, true

	case *syntax.LambdaExpr:
		if len(actual.Params) > 0 {
			return nil, false
		}

		inner, ok := actual.Body.(*syntax.CallExpr)
		if !ok {
			return nil, false
		}

		return r._Call(inner, scope)
	}

	return nil, false
}

// _Both reports that a call carries args and operands, which is two lists
// for one argument list.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
func _Both(call *workflowpb.Call) bool {
	return call != nil && len(call.GetArgs()) > 0 && len(call.GetOperands()) > 0
}

// _CallExpr is a call.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the call node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
func _CallExpr(call *workflowpb.Call) (syntax.Expr, error) {
	if call == nil || call.GetFunction() == "" {
		return nil, fmt.Errorf("call: %w", ERR_FORM)
	}

	if _Both(call) {
		return nil, fmt.Errorf("%s: %w", call.GetFunction(), ERR_FORM)
	}

	parts, err := _Parts(call)
	if err != nil {
		return nil, err
	}

	return _Bare(call.GetFunction(), parts), nil
}

// _Parts is a call's argument list. Operands win when any argument is a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the argument expressions
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
func _Parts(call *workflowpb.Call) ([]syntax.Expr, error) {
	if len(call.GetOperands()) > 0 {
		return _Operands(call.GetOperands())
	}

	var parts []syntax.Expr

	for _, arg := range call.GetArgs() {
		value, err := _Value(arg)
		if err != nil {
			return nil, err
		}

		parts = append(parts, value)
	}

	return parts, nil
}

// _Operands is an argument list that names something.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the argument expressions
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
func _Operands(ops []*workflowpb.Operand) ([]syntax.Expr, error) {
	var parts []syntax.Expr

	for _, op := range ops {
		value, err := _Operand(op)
		if err != nil {
			return nil, err
		}

		parts = append(parts, value)
	}

	return parts, nil
}

// _Operand is one argument, a literal or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
func _Operand(op *workflowpb.Operand) (syntax.Expr, error) {
	if op == nil {
		return nil, fmt.Errorf("operand: %w", ERR_FORM)
	}

	switch src := op.GetSource().(type) {
	case *workflowpb.Operand_Literal:
		return _Value(src.Literal)

	case *workflowpb.Operand_Name:
		if src.Name == "" {
			return nil, fmt.Errorf("operand: %w", ERR_FORM)
		}

		return _Name(src.Name), nil

	default:
		return nil, fmt.Errorf("operand: %w", ERR_FORM)
	}
}

// _Call is a call, bound to its result when the script captures one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
func _Call(call *workflowpb.Call) (syntax.Stmt, error) {
	expr, err := _CallExpr(call)
	if err != nil {
		return nil, err
	}

	return _Emit(call.GetResult(), expr), nil
}

// _Target is a function a builtin takes: the bare name when the call passes
// nothing, and a lambda when it passes arguments.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
func _Target(call *workflowpb.Call) (syntax.Expr, error) {
	if call == nil || call.GetFunction() == "" {
		return nil, fmt.Errorf("call: %w", ERR_FORM)
	}

	if call.GetResult() != "" {
		return nil, fmt.Errorf("%s: %w", call.GetFunction(), ERR_FORM)
	}

	if len(call.GetArgs()) == 0 && len(call.GetOperands()) == 0 {
		return _Name(call.GetFunction()), nil
	}

	expr, err := _CallExpr(call)
	if err != nil {
		return nil, err
	}

	return _Lambda(expr), nil
}
