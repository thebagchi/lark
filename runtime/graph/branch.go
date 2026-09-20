package graph

import (
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

const (
	// The words a branch is written with.
	IF    = "if "
	ELIF  = "elif "
	ELSE  = "else:"
	COLON = ":"

	// MATCH is the local a match evaluates its expression into.
	//
	// Once, into a name, because an expression that is a function would
	// otherwise be called once per case - which is a different program, and one
	// that works for a pure function and misleads for anything else.
	MATCH = "_match"

	// EQUALS compares a match's expression with a case's value.
	EQUALS = " == "
)

// _If is a condition and the call each side makes.
//
// An empty branch is a skip rather than a call to nothing, so an If with no
// else has no else.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func (g *_Gen) _If(branch *workflowpb.If) ([]string, error) {
	condition, err := g._Condition(branch.GetCondition())
	if err != nil {
		return nil, err
	}

	taken, err := g._Taken(branch.GetThen())
	if err != nil {
		return nil, err
	}

	lines := []string{IF + condition + COLON, INDENT + taken}

	if branch.GetElse() == nil {
		return lines, nil
	}

	otherwise, err := g._Taken(branch.GetElse())
	if err != nil {
		return nil, err
	}

	return append(lines, ELSE, INDENT+otherwise), nil
}

// _Match evaluates an expression once and calls whichever case equals it.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func (g *_Gen) _Match(branch *workflowpb.Match) ([]string, error) {
	cases := branch.GetCases()
	if len(cases) == 0 && branch.GetDefault() == nil {
		return nil, nil
	}

	expression, err := g._Expression(branch.GetExpression())
	if err != nil {
		return nil, err
	}

	lines := []string{MATCH + " = " + expression}

	word := IF

	for _, item := range cases {
		taken, err := g._Taken(item.GetCall())
		if err != nil {
			return nil, err
		}

		lines = append(
			lines,
			word+MATCH+EQUALS+strconv.Quote(item.GetValue())+COLON,
			INDENT+taken,
		)

		word = ELIF
	}

	if branch.GetDefault() == nil {
		return lines, nil
	}

	fallback, err := g._Taken(branch.GetDefault())
	if err != nil {
		return nil, err
	}

	if len(cases) == 0 {
		return append(lines, fallback), nil
	}

	return append(lines, ELSE, INDENT+fallback), nil
}

// _Condition is a literal or a call, as a branch tests it.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func (g *_Gen) _Condition(condition *workflowpb.Condition) (string, error) {
	if condition.GetCall() != nil {
		return g._Taken(condition.GetCall())
	}

	if condition.GetValue() {
		return TRUE, nil
	}

	return FALSE, nil
}

// _Expression is a literal or a call, as a match evaluates it.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func (g *_Gen) _Expression(expression *workflowpb.Expression) (string, error) {
	if expression.GetCall() != nil {
		return g._Taken(expression.GetCall())
	}

	return strconv.Quote(expression.GetValue()), nil
}

// _Taken is a branch calling the call it carries.
//
// An invocation, not a site: a branch calls, it does not hand a callable to
// something else, so a branch that passes arguments passes them directly rather
// than closing over them in a lambda.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation, as a bare name
//   - 2026-09-20 21:12: takes a Call, so a branch can pass arguments
func (g *_Gen) _Taken(call *workflowpb.Call) (string, error) {
	return g._Invocation(call)
}

// _Branch is the If an if statement states, and whether both its sides are
// single calls.
//
// If.then and If.else are each one Call, so a branch of two statements states
// no If and the function holding it keeps its text. An absent else is an empty
// field, which the emitter renders as no else at all.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Branch(stmt *syntax.IfStmt) *workflowpb.Step {
	asked := r._Condition(stmt.Cond)
	if asked == nil {
		return nil
	}

	taken, ok := r._Only(stmt.True)
	if !ok {
		return nil
	}

	other, ok := r._Only(stmt.False)
	if !ok {
		return nil
	}

	return &workflowpb.Step{Action: &workflowpb.Step_If{
		If: &workflowpb.If{Condition: asked, Then: taken, Else: other},
	}}
}

// _Condition is the Condition an expression states, or nil.
//
// A call of a declared function, or one of Starlark's two boolean words.
// Anything else - a comparison, a name - states no condition this schema can
// carry.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Condition(expr syntax.Expr) *workflowpb.Condition {
	name, ok := expr.(*syntax.Ident)
	if ok {
		value, ok := _Word(name.Name)
		if !ok || value.GetStructValue() != nil {
			return nil
		}

		return &workflowpb.Condition{
			Kind: &workflowpb.Condition_Value{Value: value.GetBoolValue()},
		}
	}

	call, ok := expr.(*syntax.CallExpr)
	if !ok || r.defs[_Bare(call)] == nil || !_Stated(call) {
		return nil
	}

	return &workflowpb.Condition{
		Kind: &workflowpb.Condition_Call{
			Call: &workflowpb.Call{Function: _Bare(call), Args: _Values(call)},
		},
	}
}

// _Only is the single call a branch body is, and whether it is only that.
//
// An empty body is a skip rather than a failure, so it states no call and is
// still accepted: that is how an if with no else is carried.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Only(stmts []syntax.Stmt) (*workflowpb.Call, bool) {
	if len(stmts) == 0 {
		return nil, true
	}

	if len(stmts) != 1 {
		return nil, false
	}

	held, ok := stmts[0].(*syntax.ExprStmt)
	if !ok {
		return nil, false
	}

	call, ok := held.X.(*syntax.CallExpr)
	if !ok || r.defs[_Bare(call)] == nil || !_Stated(call) {
		return nil, false
	}

	return &workflowpb.Call{Function: _Bare(call), Args: _Values(call)}, true
}

// _Match is the Match an assignment and the chain testing it state together,
// and whether they state one.
//
// Both statements at once. Reading the assignment as a step and the chain as
// an If would be two steps for one decision, and would re-emit as a different
// program - one that calls its expression once per case.
//
// Only the shape the emitter writes: an assignment to the match local from a
// call, immediately followed by a chain comparing that same name to string
// literals. That is narrower than it looks rather than narrower than it should
// be - Expression carries a string or a Call and never a variable, and
// Case.value is a string, so an authored chain over a variable could not be a
// Match whatever this did.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Match(assign *syntax.AssignStmt, chain *syntax.IfStmt) *workflowpb.Step {
	held, ok := assign.LHS.(*syntax.Ident)
	if !ok || held.Name != MATCH || assign.Op != syntax.EQ {
		return nil
	}

	call, ok := assign.RHS.(*syntax.CallExpr)
	if !ok || r.defs[_Bare(call)] == nil || !_Stated(call) {
		return nil
	}

	match := &workflowpb.Match{
		Expression: &workflowpb.Expression{
			Kind: &workflowpb.Expression_Call{
				Call: &workflowpb.Call{Function: _Bare(call), Args: _Values(call)},
			},
		},
	}

	if !r._Cases(match, chain) {
		return nil
	}

	return &workflowpb.Step{Action: &workflowpb.Step_Match{Match: match}}
}

// _Cases fills a match from the chain testing its local, and reports whether
// every arm of that chain was one it could read.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func (r *_Reading) _Cases(match *workflowpb.Match, chain *syntax.IfStmt) bool {
	value, ok := _Tested(chain.Cond)
	if !ok {
		return false
	}

	taken, ok := r._Only(chain.True)
	if !ok || taken == nil {
		return false
	}

	match.Cases = append(match.Cases, &workflowpb.Case{Value: value, Call: taken})

	if len(chain.False) == 1 {
		deeper, ok := chain.False[0].(*syntax.IfStmt)
		if ok {
			return r._Cases(match, deeper)
		}
	}

	other, ok := r._Only(chain.False)
	if !ok {
		return false
	}

	match.Default = other

	return true
}

// _Tested is the string a chain's arm compares the match local to.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
func _Tested(expr syntax.Expr) (string, bool) {
	compared, ok := expr.(*syntax.BinaryExpr)
	if !ok || compared.Op != syntax.EQL {
		return "", false
	}

	held, ok := compared.X.(*syntax.Ident)
	if !ok || held.Name != MATCH {
		return "", false
	}

	literal, ok := compared.Y.(*syntax.Literal)
	if !ok {
		return "", false
	}

	text, ok := literal.Value.(string)

	return text, ok
}
