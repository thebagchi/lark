package graph

import (
	"fmt"

	"go.starlark.net/syntax"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// Both directions of one subject live here: reading a loop out of a script, and
// writing one back as for _ in range(times).

// BLANK is the loop variable a Loop is written with. The flow does not store
// the name, because the body does not read it.
const BLANK = "_"

// _Loop is the statement a for is, and whether it models.
//
// for <name> in range(<count>), whose count is a whole number or a name the
// scope sees, and whose body is one statement once its closer is set aside.
// The body may not read the loop variable, and may bind nothing: a name it
// bound would be one binding on the flow and one thread per iteration on the
// graph, and after the loop it holds only the last.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Loop(stmt *syntax.ForStmt, scope *_Scope) (*workflowpb.Statement, bool) {
	variable, ok := stmt.Vars.(*syntax.Ident)
	if !ok {
		return nil, false
	}

	over, ok := stmt.X.(*syntax.CallExpr)
	if !ok || _Callee(over) != RANGE || !scope._Usable(RANGE) || len(over.Args) != 1 {
		return nil, false
	}

	times, ok := r._Times(over.Args[0], scope)
	if !ok {
		return nil, false
	}

	body := dialect.Closed(stmt.Body)
	if len(body) != 1 || _Reads(body[0], variable.Name) || _Binds(body[0]) {
		return nil, false
	}

	inner, ok := r._Statement(body[0], scope._Inner())
	if !ok {
		return nil, false
	}

	return &workflowpb.Statement{Action: &workflowpb.Statement_Loop{
		Loop: &workflowpb.Loop{Times: times, Body: inner},
	}}, true
}

// _Times is the count a loop runs, as an Operand: a whole, non-negative number,
// or a name the scope sees.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Times(expr syntax.Expr, scope *_Scope) (*workflowpb.Operand, bool) {
	if name, ok := expr.(*syntax.Ident); ok {
		if _, word := _Word(name.Name); word || !scope._Sees(name.Name) {
			return nil, false
		}

		return &workflowpb.Operand{Source: &workflowpb.Operand_Name{Name: name.Name}}, true
	}

	count, ok := _Natural(expr)
	if !ok {
		return nil, false
	}

	value := structpb.NewNumberValue(float64(count))

	return &workflowpb.Operand{Source: &workflowpb.Operand_Literal{Literal: value}}, true
}

// _Reads reports whether a statement reads name anywhere inside it.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func _Reads(stmt syntax.Stmt, name string) bool {
	found := false

	syntax.Walk(stmt, func(node syntax.Node) bool {
		ident, ok := node.(*syntax.Ident)
		if ok && ident.Name == name {
			found = true
		}

		return !found
	})

	return found
}

// _Binds reports whether a statement binds a name anywhere inside it: an
// assignment, a loop variable or a def.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-10-03 20:51: through dialect.Bound, the one walk of what binds a name
func _Binds(stmt syntax.Stmt) bool {
	bound := make(map[string]bool)

	dialect.Bound([]syntax.Stmt{stmt}, bound)

	return len(bound) > 0
}

// _Loop is for _ in range(times) with one statement as the body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the for node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Loop
func _Loop(loop *workflowpb.Loop, prog *_Program) (syntax.Stmt, error) {
	if loop == nil || loop.GetBody() == nil {
		return nil, fmt.Errorf("loop: %w", ERR_NO_BODY)
	}

	times, err := _Times(loop.GetTimes())
	if err != nil {
		return nil, err
	}

	body, err := _Statement(loop.GetBody(), prog)
	if err != nil {
		return nil, err
	}

	return &syntax.ForStmt{
		Vars: _Name(BLANK),
		X:    _Bare(RANGE, []syntax.Expr{times}),
		Body: body,
	}, nil
}

// _Times is a loop count: a name, or a whole number.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
func _Times(op *workflowpb.Operand) (syntax.Expr, error) {
	if op == nil {
		return nil, fmt.Errorf("loop: %w", ERR_FORM)
	}

	switch src := op.GetSource().(type) {
	case *workflowpb.Operand_Name:
		if src.Name == "" {
			return nil, fmt.Errorf("loop: %w", ERR_FORM)
		}

		return _Name(src.Name), nil

	case *workflowpb.Operand_Literal:
		text, err := _Whole(src.Literal)
		if err != nil {
			return nil, err
		}

		return _Num(text), nil

	default:
		return nil, fmt.Errorf("loop: %w", ERR_FORM)
	}
}
