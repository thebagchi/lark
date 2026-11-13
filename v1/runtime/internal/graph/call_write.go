package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// One direction of one subject lives here: writing a flow's call back as the
// call a script would make. The other direction is in call_read.go.

// _Both reports that a call carries args and operands, which is two lists
// for one argument list.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-11-02 10:25: moved to call_write.go
func _Both(call *workflowpb.Call) bool {
	return call != nil && len(call.GetArgs()) > 0 && len(call.GetOperands()) > 0
}

// _CallExpr is a call.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the call node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
//   - 2026-11-02 10:25: moved to call_write.go
//   - 2026-11-04 19:26: writes the keywords after the other arguments (C44c,
//     B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
func _CallExpr(call *workflowpb.Call) (syntax.Expr, error) {
	if call == nil || call.GetFunction() == "" {
		return nil, fmt.Errorf(
			"call: %w",
			ErrForm,
		)
	}

	if _Both(call) {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrForm,
		)
	}

	parts, err := _Parts(call)
	if err != nil {
		return nil, err
	}

	named, err := _Keywords(call.GetKeywords())
	if err != nil {
		return nil, err
	}

	parts = append(
		parts,
		named...,
	)

	return _Bare(
		call.GetFunction(),
		parts,
	), nil
}

// _Parts is a call's argument list. Operands win when any argument is a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the argument expressions
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
//   - 2026-11-02 10:25: moved to call_write.go
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

		parts = append(
			parts,
			value,
		)
	}

	return parts, nil
}

// _Operands is an argument list that names something.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the argument expressions
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
//   - 2026-11-02 10:25: moved to call_write.go
func _Operands(ops []*workflowpb.Operand) ([]syntax.Expr, error) {
	var parts []syntax.Expr

	for _, op := range ops {
		value, err := _Operand(op)
		if err != nil {
			return nil, err
		}

		parts = append(
			parts,
			value,
		)
	}

	return parts, nil
}

// _Operand is one argument, a literal or a name.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
//   - 2026-11-02 10:25: moved to call_write.go
//   - 2026-11-04 19:26: writes an expression as the script wrote it (C44c,
//     B054)
func _Operand(op *workflowpb.Operand) (syntax.Expr, error) {
	if op == nil {
		return nil, fmt.Errorf(
			"operand: %w",
			ErrForm,
		)
	}

	switch src := op.GetSource().(type) {
	case *workflowpb.Operand_Literal:
		return _Value(src.Literal)

	case *workflowpb.Operand_Name:
		if src.Name == "" {
			return nil, fmt.Errorf(
				"operand: %w",
				ErrForm,
			)
		}

		return _Name(src.Name), nil

	case *workflowpb.Operand_Expression:
		return _Written(src.Expression)

	default:
		return nil, fmt.Errorf(
			"operand: %w",
			ErrForm,
		)
	}
}

// _Written is an argument the script wrote as an expression, which the
// script keeps as written.
//
// The text is checked to be one argument and is then written as it stands:
// a literal node whose text is the whole of it, which is how the template
// writes any literal.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-04 19:26: refuses a text that is not one argument of a call
func _Written(text string) (syntax.Expr, error) {
	if !_Alone(text) {
		return nil, fmt.Errorf(
			"expression: %w",
			ErrForm,
		)
	}

	return &syntax.Literal{
		Token: syntax.STRING,
		Raw:   text,
	}, nil
}

// _Alone reports whether text is exactly one positional argument of a call:
// one expression, with no comment that would take the closing bracket, no
// second argument after a comma, no keyword and no unpacking, and nothing
// that closes the call early.
//
// Parsed where it is written, between the brackets of a call. A multi-line
// expression such as x > then 1 on the next line is valid there and not as a
// text of its own, so a parse of the text alone would refuse what the script
// wrote.
//
// The text is whole only when the one call the parse made is the wrapper
// itself: its callee is the wrapper's name, and its closing bracket is the
// last character. So 1)(2 and a)(b, which parse as a call of a call, 1) + (2,
// and a text that adds a suffix after the bracket it closes, are refused.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-04 19:26: requires the wrapper to be the whole parse, so text that
//     closes the call early is refused
//   - 2026-11-05 23:26: _Alone finds the place with NewLines(...).At in place of
//     _NewLines(...)._At (C60, Q188)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func _Alone(text string) bool {
	const WRAPPER = "_"

	var wrapped = WRAPPER + "(" + text + ")"

	expr, err := common.OPTIONS.ParseExpr(
		EXPRESSION,
		wrapped,
		0,
	)
	if err != nil {
		return false
	}

	call, ok := expr.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}

	fn, ok := call.Fn.(*syntax.Ident)
	if !ok || fn.Name != WRAPPER {
		return false
	}

	if NewLines(wrapped).At(
		call.Rparen.Line,
		call.Rparen.Col,
	) != len(wrapped)-1 {
		return false
	}

	switch actual := call.Args[0].(type) {
	case *syntax.UnaryExpr:
		return actual.Op != syntax.STAR && actual.Op != syntax.STARSTAR

	case *syntax.BinaryExpr:
		return actual.Op != syntax.EQ
	}

	return true
}

// _Keywords is the arguments a call passes by name, as name = value.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
func _Keywords(list []*workflowpb.Keyword) ([]syntax.Expr, error) {
	var parts []syntax.Expr

	for _, keyword := range list {
		if keyword.GetName() == "" {
			return nil, fmt.Errorf(
				"keyword: %w",
				ErrForm,
			)
		}

		value, err := _Operand(keyword.GetValue())
		if err != nil {
			return nil, err
		}

		parts = append(
			parts,
			&syntax.BinaryExpr{
				X:  _Name(keyword.GetName()),
				Op: syntax.EQ,
				Y:  value,
			},
		)
	}

	return parts, nil
}

// Call is a call, bound to its result when the script captures one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
//   - 2026-11-02 10:25: moved to call_write.go
//   - 2026-11-10 01:00: comment now names Call, were _Call (C120a, Q314)
func _Call(call *workflowpb.Call) (syntax.Stmt, error) {
	expr, err := _CallExpr(call)
	if err != nil {
		return nil, err
	}

	return _Emit(
		call.GetResult(),
		expr,
	), nil
}

// Target is a function a builtin takes: the bare name when the call passes
// nothing, and a lambda when it passes arguments.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Call
//   - 2026-11-02 10:25: moved to call_write.go
//   - 2026-11-04 19:26: a call with a keyword is a lambda too (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: comment now names Target, were _Target (C120a, Q314)
func _Target(call *workflowpb.Call) (syntax.Expr, error) {
	if call == nil || call.GetFunction() == "" {
		return nil, fmt.Errorf(
			"call: %w",
			ErrForm,
		)
	}

	if call.GetResult() != "" {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrForm,
		)
	}

	if len(call.GetArgs()) == 0 && len(call.GetOperands()) == 0 &&
		len(call.GetKeywords()) == 0 {
		return _Name(call.GetFunction()), nil
	}

	expr, err := _CallExpr(call)
	if err != nil {
		return nil, err
	}

	return _Lambda(expr), nil
}
