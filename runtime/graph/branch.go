package graph

import (
	"strconv"

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
