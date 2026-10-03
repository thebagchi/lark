package graph

import (
	"fmt"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Visible is what one statement list of a flow can see while Check walks it:
// the function's parameters, the results and spawn bindings earlier on this
// list and on every list around it, and the functions, constants and arguments
// the flow declares.
//
// A type of its own rather than derive's _Scope, because a flow arrives with
// its names chosen by whoever authored it: there is no syntax, and no local a
// Starlark function binds elsewhere, to account for. A list inside a statement
// sees the list around it, and what it binds does not leak out.
type _Visible struct {
	outer   *_Visible
	globals map[string]bool
	params  map[string]bool
	values  map[string]bool
	spawns  map[string]bool
}

// _Opening is what a function's own statement list sees before its first
// statement.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Opening(globals map[string]bool, params []string) *_Visible {
	seen := &_Visible{
		globals: globals,
		params:  make(map[string]bool),
		values:  make(map[string]bool),
		spawns:  make(map[string]bool),
	}

	for _, name := range params {
		seen.params[name] = true
	}

	return seen
}

// _Inner is what a list inside a statement on this one sees.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Inner() *_Visible {
	return &_Visible{
		outer:   v,
		globals: v.globals,
		params:  v.params,
		values:  make(map[string]bool),
		spawns:  make(map[string]bool),
	}
}

// _Sees reports whether name resolves here: a parameter, a result or a spawn
// binding this list or one around it holds, or what the flow declares.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Sees(name string) bool {
	if v.params[name] || v.globals[name] {
		return true
	}

	for seen := v; seen != nil; seen = seen.outer {
		if seen.values[name] || seen.spawns[name] {
			return true
		}
	}

	return false
}

// _Spawned reports whether name is a spawn binding this list or one around it
// holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Spawned(name string) bool {
	for seen := v; seen != nil; seen = seen.outer {
		if seen.spawns[name] {
			return true
		}
	}

	return false
}

// _List checks a statement list in order, each statement seeing what the ones
// before it bound.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _List(list []*workflowpb.Statement) error {
	for _, stmt := range list {
		err := v._Statement(stmt)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Statement checks one statement's names, and records what it binds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Statement(stmt *workflowpb.Statement) error {
	if stmt == nil || stmt.GetAction() == nil {
		return fmt.Errorf("statement: %w", ERR_FORM)
	}

	switch action := stmt.GetAction().(type) {
	case *workflowpb.Statement_Call:
		return v._Result(action.Call)

	case *workflowpb.Statement_Spawn:
		return v._Spawn(action.Spawn)

	case *workflowpb.Statement_Join:
		err := v._Names(action.Join.GetBindings())
		if err != nil {
			return err
		}

		v._Bind(action.Join.GetResult())

		return nil

	case *workflowpb.Statement_Cancel:
		return v._Names(action.Cancel.GetBindings())

	case *workflowpb.Statement_If:
		return v._If(action.If)

	case *workflowpb.Statement_Match:
		return v._Match(action.Match)

	case *workflowpb.Statement_Loop:
		return v._Loop(action.Loop)
	}

	return v._Operands(_Made(stmt))
}

// _Result checks a call's names and binds its result.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Result(call *workflowpb.Call) error {
	err := v._Operands([]*workflowpb.Call{call})
	if err != nil {
		return err
	}

	v._Bind(call.GetResult())

	return nil
}

// _Spawn checks a spawn's call, and records its binding: a second spawn of one
// binding on one list is refused.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Spawn(spawn *workflowpb.Spawn) error {
	err := v._Operands([]*workflowpb.Call{spawn.GetCall()})
	if err != nil {
		return err
	}

	binding := spawn.GetBinding()
	if binding == "" {
		return nil
	}

	if v.spawns[binding] {
		return fmt.Errorf("two spawns bound to %s: %w", binding, ERR_NOT_FORKED)
	}

	v.spawns[binding] = true

	return nil
}

// _Names checks that every binding a join or a cancel names is a spawn this
// list or one around it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Names(bindings []string) error {
	for _, binding := range bindings {
		if !v._Spawned(binding) {
			return fmt.Errorf("%s: %w", binding, ERR_NOT_FORKED)
		}
	}

	return nil
}

// _If checks an if: its condition, and each branch, which sees this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _If(branch *workflowpb.If) error {
	condition := branch.GetCondition()

	err := v._Name(condition.GetName())
	if err != nil {
		return err
	}

	err = v._Operands([]*workflowpb.Call{condition.GetCall()})
	if err != nil {
		return err
	}

	if branch.GetThen() == nil {
		return fmt.Errorf("if: %w", ERR_NO_BODY)
	}

	err = v._Inner()._Statement(branch.GetThen())
	if err != nil {
		return err
	}

	if branch.GetElse() == nil {
		return nil
	}

	return v._Inner()._Statement(branch.GetElse())
}

// _Match checks a match: its expression, and each case and the default, which
// see this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Match(match *workflowpb.Match) error {
	expression := match.GetExpression()

	err := v._Name(expression.GetName())
	if err != nil {
		return err
	}

	err = v._Operands([]*workflowpb.Call{expression.GetCall()})
	if err != nil {
		return err
	}

	for _, arm := range match.GetCases() {
		err = v._Inner()._Statement(arm.GetStatement())
		if err != nil {
			return err
		}
	}

	if match.GetDefault() == nil {
		return nil
	}

	return v._Inner()._Statement(match.GetDefault())
}

// _Loop checks a loop: its count, and its body, which sees this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Loop(loop *workflowpb.Loop) error {
	err := v._Name(loop.GetTimes().GetName())
	if err != nil {
		return err
	}

	if loop.GetBody() == nil {
		return fmt.Errorf("loop: %w", ERR_NO_BODY)
	}

	return v._Inner()._Statement(loop.GetBody())
}

// _Operands checks that every name these calls pass resolves here.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Operands(calls []*workflowpb.Call) error {
	for _, call := range calls {
		for _, op := range call.GetOperands() {
			err := v._Name(op.GetName())
			if err != nil {
				return fmt.Errorf("%s passes %w", call.GetFunction(), err)
			}
		}
	}

	return nil
}

// _Name checks that name, when there is one, resolves here.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Name(name string) error {
	if name == "" || v._Sees(name) {
		return nil
	}

	return fmt.Errorf("%s: %w", name, ERR_UNRESOLVED)
}

// _Bind records a result this list binds, when there is one.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func (v *_Visible) _Bind(name string) {
	if name != "" {
		v.values[name] = true
	}
}
