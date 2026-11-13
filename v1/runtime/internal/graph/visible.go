package graph

import (
	"fmt"
	"slices"
	"sync"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Visible is what one statement list of a flow can see while Check walks it:
// the function's parameters, the results and spawn bindings earlier on this
// list and on every list around it, and the functions, constants and arguments
// the flow declares.
//
// A type of its own rather than derive's Scope, because a flow arrives with
// its names chosen by whoever authored it: there is no syntax, and no local a
// Starlark function binds elsewhere, to account for. A list inside a statement
// sees the list around it, and what it binds does not leak out.
//
// guard holds every write to and read of values and spawns, so goroutines that
// bind on one list do not race. A list inside a statement shares params with
// the list around it, so home names the visible set whose guard holds them.
// globals belongs to the caller, who owns its locking.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 10:35: the guard and home fields make writes to the maps safe
//     (C108, B194, Q207)
//   - 2026-11-10 00:25: renamed from _Visible and the fields guard, params, spawns, values, as a
//     test in tests/ needs them (C120a, Q314)
type Visible struct {
	Guard   sync.Mutex
	outer   *Visible
	home    *Visible
	globals map[string]bool
	Params  map[string]bool
	Values  map[string]bool
	Spawns  map[string]bool
}

// _Opening is what a function's own statement list sees before its first
// statement.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-10 01:00: now uses Visible, Params, Values, Spawns, were _Visible, params, values,
//     spawns (C120a, Q314)
func _Opening(
	globals map[string]bool,
	params []string,
) *Visible {
	seen := &Visible{
		globals: globals,
		Params:  make(map[string]bool),
		Values:  make(map[string]bool),
		Spawns:  make(map[string]bool),
	}

	for _, name := range params {
		seen.Params[name] = true
	}

	return seen
}

// Inner is what a list inside a statement on this one sees.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 12:59: names the set that owns the shared params (C108, B194, Q207)
//   - 2026-11-10 00:25: renamed from _Inner, as a test in tests/ needs it (C120a, Q314)
func (v *Visible) Inner() *Visible {
	return &Visible{
		outer:   v,
		home:    v._Home(),
		globals: v.globals,
		Params:  v.Params,
		Values:  make(map[string]bool),
		Spawns:  make(map[string]bool),
	}
}

// Sees reports whether name resolves here: a parameter, a result or a spawn
// binding this list or one around it holds, or what the flow declares.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 10:35: reads the maps under the guard (C108, B194)
//   - 2026-11-10 00:25: renamed from _Sees, as a test in tests/ needs it (C120a, Q314)
func (v *Visible) Sees(name string) bool {
	if v._Param(name) || v.globals[name] {
		return true
	}

	for seen := v; seen != nil; seen = seen.outer {
		if seen.Holds(
			name,
			true,
		) {
			return true
		}
	}

	return false
}

// Holds reports whether this list alone holds name as a spawn binding, or as
// a result as well when values is true.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B194)
//   - 2026-11-10 00:25: renamed from _Holds, as a test in tests/ needs it (C120a, Q314)
func (v *Visible) Holds(
	name string,
	values bool,
) bool {
	v.Guard.Lock()
	defer v.Guard.Unlock()

	return v.Spawns[name] || values && v.Values[name]
}

// Spawned reports whether name is a spawn binding this list or one around it
// holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 10:35: reads the map under the guard (C108, B194)
//   - 2026-11-10 00:25: renamed from _Spawned, as a test in tests/ needs it (C120a, Q314)
func (v *Visible) Spawned(name string) bool {
	for seen := v; seen != nil; seen = seen.outer {
		if seen.Holds(
			name,
			false,
		) {
			return true
		}
	}

	return false
}

// Spawn checks a spawn's call, and records its binding: a second spawn of one
// binding on one list is refused.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 10:35: tests and sets the binding under the guard, and makes a missing
//     map (C108, B193, B194)
//   - 2026-11-10 00:25: renamed from _Spawn, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrFork, formerly ErrNotForked (C121, Q348)
func (v *Visible) Spawn(spawn *workflowpb.Spawn) error {
	err := v._Operands([]*workflowpb.Call{spawn.GetCall()})
	if err != nil {
		return err
	}

	binding := spawn.GetBinding()
	if binding == "" {
		return nil
	}

	v.Guard.Lock()
	defer v.Guard.Unlock()

	if v.Spawns[binding] {
		return fmt.Errorf(
			"two spawns bound to %s: %w",
			unpack.QuotePrefix(binding),
			ErrFork,
		)
	}

	if v.Spawns == nil {
		v.Spawns = make(map[string]bool)
	}

	v.Spawns[binding] = true

	return nil
}

// Bind records a result this list binds, when there is one.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 10:35: writes under the guard, and makes a missing map
//     (C108, B194, B223)
//   - 2026-11-10 00:25: renamed from _Bind, as a test in tests/ needs it (C120a, Q314)
func (v *Visible) Bind(name string) {
	if name == "" {
		return
	}

	v.Guard.Lock()
	defer v.Guard.Unlock()

	if v.Values == nil {
		v.Values = make(map[string]bool)
	}

	v.Values[name] = true
}

// _Home is the visible set whose guard holds the params this one reads.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B194, Q207)
//   - 2026-11-10 01:00: now uses Visible, were _Visible (C120a, Q314)
func (v *Visible) _Home() *Visible {
	if v.home != nil {
		return v.home
	}

	return v
}

// _Param reports whether name is a parameter of the function, read under the
// guard of the set that owns the params.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B194, Q207)
//   - 2026-11-10 01:00: now uses Visible, Guard, Params, were _Visible, guard, params
//     (C120a, Q314)
func (v *Visible) _Param(name string) bool {
	home := v._Home()

	home.Guard.Lock()
	defer home.Guard.Unlock()

	return v.Params[name]
}

// _List checks a statement list in order, each statement seeing what the ones
// before it bound.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-10 01:00: now uses Visible, were _Visible (C120a, Q314)
func (v *Visible) _List(list []*workflowpb.Statement) error {
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
//   - 2026-11-10 01:00: now uses Visible, Spawn, Bind, were _Visible, _Spawn, _Bind (C120a, Q314)
func (v *Visible) _Statement(stmt *workflowpb.Statement) error {
	if stmt == nil || stmt.GetAction() == nil {
		return fmt.Errorf(
			"statement: %w",
			ErrForm,
		)
	}

	switch action := stmt.GetAction().(type) {
	case *workflowpb.Statement_Call:
		return v._Result(action.Call)

	case *workflowpb.Statement_Spawn:
		return v.Spawn(action.Spawn)

	case *workflowpb.Statement_Join:
		err := v._Names(action.Join.GetBindings())
		if err != nil {
			return err
		}

		v.Bind(action.Join.GetResult())

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
//   - 2026-11-10 01:00: now uses Visible, Bind, were _Visible, _Bind (C120a, Q314)
func (v *Visible) _Result(call *workflowpb.Call) error {
	err := v._Operands([]*workflowpb.Call{call})
	if err != nil {
		return err
	}

	v.Bind(call.GetResult())

	return nil
}

// _Names checks that every binding a join or a cancel names is a spawn this
// list or one around it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: now uses Visible, Spawned, were _Visible, _Spawned (C120a, Q314)
//   - 2026-11-10 17:15: uses ErrFork, formerly ErrNotForked (C121, Q348)
func (v *Visible) _Names(bindings []string) error {
	for _, binding := range bindings {
		if !v.Spawned(binding) {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(binding),
				ErrFork,
			)
		}
	}

	return nil
}

// If checks an if: its condition, and each branch, which sees this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses If, Visible, Inner, were _If, _Visible, _Inner (C120a, Q314)
func (v *Visible) _If(branch *workflowpb.If) error {
	var condition = branch.GetCondition()

	err := v._Name(condition.GetName())
	if err != nil {
		return err
	}

	err = v._Operands([]*workflowpb.Call{condition.GetCall()})
	if err != nil {
		return err
	}

	if branch.GetThen() == nil {
		return fmt.Errorf(
			"if: function has no body: %w",
			ErrForm,
		)
	}

	err = v.Inner()._Statement(branch.GetThen())
	if err != nil {
		return err
	}

	if branch.GetElse() == nil {
		return nil
	}

	return v.Inner()._Statement(branch.GetElse())
}

// _Match checks a match: its expression, and each case and the default, which
// see this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-10 01:00: now uses Visible, Inner, were _Visible, _Inner (C120a, Q314)
func (v *Visible) _Match(match *workflowpb.Match) error {
	var expression = match.GetExpression()

	err := v._Name(expression.GetName())
	if err != nil {
		return err
	}

	err = v._Operands([]*workflowpb.Call{expression.GetCall()})
	if err != nil {
		return err
	}

	for _, arm := range match.GetCases() {
		err = v.Inner()._Statement(arm.GetStatement())
		if err != nil {
			return err
		}
	}

	if match.GetDefault() == nil {
		return nil
	}

	return v.Inner()._Statement(match.GetDefault())
}

// Loop checks a loop: its count, and its body, which sees this list.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Loop, Visible, Inner, were _Loop, _Visible, _Inner (C120a, Q314)
func (v *Visible) _Loop(loop *workflowpb.Loop) error {
	err := v._Name(loop.GetTimes().GetName())
	if err != nil {
		return err
	}

	if loop.GetBody() == nil {
		return fmt.Errorf(
			"loop: function has no body: %w",
			ErrForm,
		)
	}

	return v.Inner()._Statement(loop.GetBody())
}

// _Operands checks that every name these calls pass resolves here.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-04 19:26: the values of keywords too (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: now uses Visible, were _Visible (C120a, Q314)
func (v *Visible) _Operands(calls []*workflowpb.Call) error {
	for _, call := range calls {
		passed := slices.Clone(call.GetOperands())

		for _, keyword := range call.GetKeywords() {
			passed = append(
				passed,
				keyword.GetValue(),
			)
		}

		for _, op := range passed {
			err := v._Name(op.GetName())
			if err != nil {
				return fmt.Errorf(
					"%s passes %w",
					unpack.QuotePrefix(call.GetFunction()),
					err,
				)
			}
		}
	}

	return nil
}

// _Name checks that name, when there is one, resolves here.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-10 01:00: now uses Visible, Sees, were _Visible, _Sees (C120a, Q314)
func (v *Visible) _Name(name string) error {
	if name == "" || v.Sees(name) {
		return nil
	}

	return fmt.Errorf(
		"%s: %w",
		unpack.QuotePrefix(name),
		ErrUnresolved,
	)
}
