package graph

import (
	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// One direction of one subject lives here: reading a call of a listed function
// out of a script. The other direction is in call_write.go.

// Call is the call a script makes of a function the flow lists, and whether
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
//   - 2026-11-02 10:25: moved to call_read.go
//   - 2026-11-10 00:25: renamed from _Call, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Call(
	call *syntax.CallExpr,
	scope *Scope,
) (*workflowpb.Call, bool) {
	name := _Callee(call)
	if !scope.Callable(name) {
		return nil, false
	}

	args, operands, ok := _Carries(
		call.Args,
		scope,
	)
	if !ok {
		return nil, false
	}

	return &workflowpb.Call{
		Function: name,
		Args:     args,
		Operands: operands,
	}, true
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
//   - 2026-11-02 10:25: moved to call_read.go
//   - 2026-11-10 01:00: now uses Scope, were _Scope (C120a, Q314)
//   - 2026-11-13 10:16: the args are workflow.Value (C132, Q407, Q425)
func _Carries(
	exprs []syntax.Expr,
	scope *Scope,
) ([]*workflowpb.Value, []*workflowpb.Operand, bool) {
	var (
		operands []*workflowpb.Operand
		values   []*workflowpb.Value
		named    bool
	)

	for _, expr := range exprs {
		operand, ok := _Carried(
			expr,
			scope,
		)
		if !ok {
			return nil, nil, false
		}

		operands = append(
			operands,
			operand,
		)
		values = append(
			values,
			operand.GetLiteral(),
		)
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
//   - 2026-11-02 10:25: moved to call_read.go
//   - 2026-11-10 01:00: now uses Scope, Sees, were _Scope, _Sees (C120a, Q314)
func _Carried(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Operand, bool) {
	value, ok := _Arg(expr)
	if ok {
		return &workflowpb.Operand{
			Source: &workflowpb.Operand_Literal{
				Literal: value,
			},
		}, true
	}

	name, ok := expr.(*syntax.Ident)
	if !ok || !scope.Sees(name.Name) {
		return nil, false
	}

	return &workflowpb.Operand{
		Source: &workflowpb.Operand_Name{
			Name: name.Name,
		},
	}, true
}

// Target is the call a builtin taking a function makes, and whether the flow
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
//   - 2026-11-02 10:25: moved to call_read.go
//   - 2026-11-10 00:25: renamed from _Target, as a test in tests/ needs it (C120a, Q314)
func (r *Reading) Target(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Call, bool) {
	switch actual := expr.(type) {
	case *syntax.Ident:
		if !scope.Callable(actual.Name) {
			return nil, false
		}

		return &workflowpb.Call{
			Function: actual.Name,
		}, true

	case *syntax.LambdaExpr:
		if len(actual.Params) > 0 {
			return nil, false
		}

		inner, ok := actual.Body.(*syntax.CallExpr)
		if !ok {
			return nil, false
		}

		return r.Call(
			inner,
			scope,
		)
	}

	return nil, false
}

// _Extended is the call a script makes of any callee, and whether the flow can
// carry it.
//
// A function the flow lists is a call as Call reads it. A builtin, a
// predeclared name, or a member of one such as hex.from_bytes is a call with
// builtin set, unless the script declares the same name itself. An argument may
// be a keyword, and one that is no value or name is carried as the expression
// the script wrote. An unpacked *args or **kwargs is not carried.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Call, Reading, Scope, were _Call, _Reading, _Scope (C120a, Q314)
func (r *Reading) _Extended(
	call *syntax.CallExpr,
	scope *Scope,
) (*workflowpb.Call, bool) {
	name, builtin, ok := r._Named(
		call.Fn,
		scope,
	)
	if !ok {
		return nil, false
	}

	held := &workflowpb.Call{
		Function: name,
		Builtin:  builtin,
	}

	var operands []*workflowpb.Operand

	for _, arg := range call.Args {
		operand, keyword, ok := _Offered(
			arg,
			scope,
		)
		if !ok {
			return nil, false
		}

		if keyword != nil {
			held.Keywords = append(
				held.Keywords,
				keyword,
			)

			continue
		}

		operands = append(
			operands,
			operand,
		)
	}

	held.Args, held.Operands = _Gathered(operands)

	return held, true
}

// _Named is the name a call's callee spells, whether it is a builtin, and
// whether the flow can carry the call at all.
//
// A bare name the flow lists is a function. A bare name, or a dotted path
// from a bare name, that the script declares nothing over is a builtin.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Reading, Scope, Callable, Usable, were _Reading, _Scope,
//     _Callable, _Usable (C120a, Q314)
func (r *Reading) _Named(
	fn syntax.Expr,
	scope *Scope,
) (string, bool, bool) {
	switch actual := fn.(type) {
	case *syntax.Ident:
		if scope.Callable(actual.Name) {
			return actual.Name, false, true
		}

		return actual.Name, true, scope.Usable(actual.Name)

	case *syntax.DotExpr:
		path, root, ok := _Path(actual)

		return path, true, ok && scope.Usable(root)
	}

	return "", false, false
}

// _Path is a dotted name such as hex.from_bytes, and the name it starts from.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
func _Path(dot *syntax.DotExpr) (string, string, bool) {
	switch actual := dot.X.(type) {
	case *syntax.Ident:
		return actual.Name + "." + dot.Name.Name, actual.Name, true

	case *syntax.DotExpr:
		path, root, ok := _Path(actual)

		return path + "." + dot.Name.Name, root, ok
	}

	return "", "", false
}

// _Offered is one argument of a call: a keyword, or an operand.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Scope, were _Scope (C120a, Q314)
func _Offered(
	arg syntax.Expr,
	scope *Scope,
) (*workflowpb.Operand, *workflowpb.Keyword, bool) {
	switch actual := arg.(type) {
	case *syntax.UnaryExpr:
		if actual.Op == syntax.STAR || actual.Op == syntax.STARSTAR {
			return nil, nil, false
		}

	case *syntax.BinaryExpr:
		name, named := actual.X.(*syntax.Ident)
		if actual.Op == syntax.EQ && named {
			value, ok := _Operanded(
				actual.Y,
				scope,
			)

			return nil, &workflowpb.Keyword{
				Name:  name.Name,
				Value: value,
			}, ok
		}
	}

	operand, ok := _Operanded(
		arg,
		scope,
	)

	return operand, nil, ok
}

// _Operanded is one argument as a value, a name the scope sees, or the
// expression the script wrote.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-10 01:00: now uses Scope, were _Scope (C120a, Q314)
func _Operanded(
	expr syntax.Expr,
	scope *Scope,
) (*workflowpb.Operand, bool) {
	operand, ok := _Carried(
		expr,
		scope,
	)
	if ok {
		return operand, true
	}

	text, ok := scope._Written(expr)
	if !ok {
		return nil, false
	}

	return &workflowpb.Operand{
		Source: &workflowpb.Operand_Expression{
			Expression: text,
		},
	}, true
}

// _Gathered is the arguments of a call as _Carries keeps them: values as args,
// or operands when any of them names or computes something.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-13 10:16: the args are workflow.Value (C132, Q407, Q425)
func _Gathered(operands []*workflowpb.Operand) ([]*workflowpb.Value, []*workflowpb.Operand) {
	var values []*workflowpb.Value

	for _, operand := range operands {
		if operand.GetLiteral() == nil {
			return nil, operands
		}

		values = append(
			values,
			operand.GetLiteral(),
		)
	}

	return values, nil
}

// _Written is the text of an expression as the script wrote it, and whether
// this scope holds the lines to cut it from.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-05 23:24: cuts with Lines.At in place of _At (C60, Q188)
//   - 2026-11-10 01:00: now uses Scope, were _Scope (C120a, Q314)
func (s *Scope) _Written(expr syntax.Expr) (string, bool) {
	if s.lines == nil {
		return "", false
	}

	var (
		start, _ = expr.Span()
		end      = _End(expr)
		from     = s.lines.At(
			start.Line,
			start.Col,
		)
		to = s.lines.At(
			end.Line,
			end.Col,
		)
	)
	if from < 0 || to < from || !_Alone(s.lines.src[from:to]) {
		return "", false
	}

	return s.lines.src[from:to], true
}
