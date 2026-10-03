package graph

import (
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// _Constants is every constant, each after the constants it names.
//
// Names that are ready together are written alphabetically, and a name that
// becomes ready while that group is being written waits for the next group.
// region is written with the other literals, and banner, which names region,
// follows the whole group.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the assignments
//   - 2026-10-02 00:04: lifted into graph, taking the generated types
func _Constants(flow *workflowpb.Flow) ([]syntax.Stmt, error) {
	names, err := _Order(flow)
	if err != nil {
		return nil, err
	}

	var stmts []syntax.Stmt

	for _, name := range names {
		bound, err := _Bound(name, flow.GetConstants()[name])
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, _Emit(name, bound))
	}

	return stmts, nil
}

// _Order is the constant names in the order _Constants writes them: in rounds,
// each the names whose constants name only constants already written, sorted
// by name. Constants that name each other never come round, and are refused.
//
// Each constant keeps a count of the constants it still waits for, and each
// name the constants waiting on it, so writing a name touches only those, and a
// round is the constants the round before brought to nothing. Taking every
// written name out of every constant's set, and reading every name each round,
// cost the square of the constants, and of a chain of them.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-02 00:04: lifted into graph, taking the generated types
//   - 2026-10-03 23:03: counts what each constant still waits for, so writing a name
//     touches only the constants that named it
func _Order(flow *workflowpb.Flow) ([]string, error) {
	held := flow.GetConstants()

	// Sorted, so the first round is alphabetical as every round after it is.
	names := slices.Sorted(maps.Keys(held))
	waits, waiters := _Waits(held, names)

	var round []string

	for _, name := range names {
		if waits[name] == 0 {
			round = append(round, name)
		}
	}

	ordered := make([]string, 0, len(names))

	for len(round) > 0 {
		ordered = append(ordered, round...)
		round = _Next(round, waits, waiters)
	}

	if len(ordered) < len(names) {
		return nil, fmt.Errorf("constants: %w", ERR_FORM)
	}

	return ordered, nil
}

// _Waits is how many constants each constant names, and for each name the
// constants that name it. A call's function is not one of them: functions are
// written before constants. A name that is not a constant is not one either,
// and a constant named twice is waited for once.
//
// Revisions:
//   - 2026-10-03 23:03: initial creation, in place of a set of names for each constant
func _Waits(
	held map[string]*workflowpb.Constant,
	names []string,
) (map[string]int, map[string][]string) {
	waits := make(map[string]int, len(names))
	waiters := make(map[string][]string)

	for _, name := range names {
		var named []string

		for _, op := range held[name].GetCall().GetOperands() {
			dep := op.GetName()
			if _, ok := held[dep]; ok {
				named = append(named, dep)
			}
		}

		slices.Sort(named)
		named = slices.Compact(named)

		waits[name] = len(named)

		for _, dep := range named {
			waiters[dep] = append(waiters[dep], name)
		}
	}

	return waits, waiters
}

// _Next is the round after round: the constants whose last wait was a name in
// round, sorted by name. A constant that becomes ready while round is written
// waits for this one, rather than joining round.
//
// Revisions:
//   - 2026-10-03 23:03: initial creation, in place of reading every name each round
func _Next(round []string, waits map[string]int, waiters map[string][]string) []string {
	var next []string

	for _, name := range round {
		for _, waiter := range waiters[name] {
			waits[waiter]--

			if waits[waiter] == 0 {
				next = append(next, waiter)
			}
		}
	}

	slices.Sort(next)

	return next
}

// _Bound is what a constant is bound to.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated types
func _Bound(name string, held *workflowpb.Constant) (syntax.Expr, error) {
	if name == "" || held == nil {
		return nil, fmt.Errorf("constant: %w", ERR_FORM)
	}

	switch kind := held.GetKind().(type) {
	case *workflowpb.Constant_Value:
		return _Value(kind.Value)

	case *workflowpb.Constant_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf("%s: %w", name, ERR_FORM)
		}

		if _Both(kind.Call) {
			return nil, fmt.Errorf("%s: %w", name, ERR_FORM)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf("%s: %w", name, ERR_FORM)
	}
}

// _Arguments is every argument, sorted by the name it binds.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the assignments
//   - 2026-10-02 00:04: lifted into graph, taking the generated types
func _Arguments(flow *workflowpb.Flow) ([]syntax.Stmt, error) {
	var stmts []syntax.Stmt

	for _, name := range slices.Sorted(maps.Keys(flow.GetArgs())) {
		if name == "" {
			return nil, fmt.Errorf("arg: %w", ERR_FORM)
		}

		call, err := _Declaration(flow.GetArgs()[name])
		if err != nil {
			return nil, err
		}

		stmts = append(stmts, _Emit(name, call))
	}

	return stmts, nil
}
