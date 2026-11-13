package graph

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Constants is every constant, each after the constants it names.
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
//   - 2026-11-10 01:00: comment now names Constants, were _Constants (C120a, Q314)
func _Constants(flow *workflowpb.Flow) ([]syntax.Stmt, error) {
	names, err := _Order(flow)
	if err != nil {
		return nil, err
	}

	var stmts []syntax.Stmt

	for _, name := range names {
		bound, err := _Bound(
			name,
			flow.GetConstants()[name],
		)
		if err != nil {
			return nil, err
		}

		stmts = append(
			stmts,
			_Emit(
				name,
				bound,
			),
		)
	}

	return stmts, nil
}

// _Order is the constant names in the order Constants writes them: in rounds,
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
//   - 2026-11-08 10:35: reads the counts through _Counts (C108, B220)
//   - 2026-11-10 01:00: now uses Constants, Next, were _Constants, _Next (C120a, Q314)
func _Order(flow *workflowpb.Flow) ([]string, error) {
	held := flow.GetConstants()

	// Sorted, so the first round is alphabetical as every round after it is.
	var names = slices.Sorted(maps.Keys(held))

	waits, waiters := _Waits(
		held,
		names,
	)

	var round []string

	for _, name := range names {
		if waits._Count(name) == 0 {
			round = append(
				round,
				name,
			)
		}
	}

	ordered := make(
		[]string,
		0,
		len(names),
	)

	for len(round) > 0 {
		ordered = append(
			ordered,
			round...,
		)
		round = Next(
			round,
			waits,
			waiters,
		)
	}

	if len(ordered) < len(names) {
		return nil, fmt.Errorf(
			"constants: %w",
			ErrForm,
		)
	}

	return ordered, nil
}

// Counts is how many constants each constant still waits for.
//
// guard holds every change to held, so two goroutines that take names off the
// same counts do not race, and each count reaches nothing once.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation, in place of a bare map (C108, B219, B220, Q207)
//   - 2026-11-10 00:25: renamed from _Counts and the fields held, as a test in tests/ needs them
//     (C120a, Q314)
type Counts struct {
	guard sync.Mutex
	Held  map[string]int
}

// _Count is how many constants name still waits for.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B220)
//   - 2026-11-10 01:00: now uses Counts, Held, were _Counts, held (C120a, Q314)
func (c *Counts) _Count(name string) int {
	c.guard.Lock()
	defer c.guard.Unlock()

	return c.Held[name]
}

// _Release takes one wait off name, and reports whether that was its last.
// Counts with no map hold nothing, so nothing is last.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B219, B220)
//   - 2026-11-10 01:00: now uses Counts, Held, were _Counts, held (C120a, Q314)
func (c *Counts) _Release(name string) bool {
	if c == nil {
		return false
	}

	c.guard.Lock()
	defer c.guard.Unlock()

	if c.Held == nil {
		return false
	}

	c.Held[name]--

	return c.Held[name] == 0
}

// _Waits is how many constants each constant names, and for each name the
// constants that name it. A call's function is not one of them: functions are
// written before constants. A name that is not a constant is not one either,
// and a constant named twice is waited for once.
//
// Revisions:
//   - 2026-10-03 23:03: initial creation, in place of a set of names for each constant
//   - 2026-11-08 10:35: returns the counts as a _Counts (C108, B220)
//   - 2026-11-10 01:00: now uses Counts, Held, were _Counts, held (C120a, Q314)
func _Waits(
	held map[string]*workflowpb.Constant,
	names []string,
) (*Counts, map[string][]string) {
	var (
		waits = &Counts{
			Held: make(
				map[string]int,
				len(names),
			),
		}
		waiters = make(map[string][]string)
	)

	for _, name := range names {
		var named []string

		for _, op := range held[name].GetCall().GetOperands() {
			dep := op.GetName()
			if _, ok := held[dep]; ok {
				named = append(
					named,
					dep,
				)
			}
		}

		slices.Sort(named)
		named = slices.Compact(named)

		waits.Held[name] = len(named)

		for _, dep := range named {
			waiters[dep] = append(
				waiters[dep],
				name,
			)
		}
	}

	return waits, waiters
}

// Next is the round after round: the constants whose last wait was a name in
// round, sorted by name. A constant that becomes ready while round is written
// waits for this one, rather than joining round.
//
// Revisions:
//   - 2026-10-03 23:03: initial creation, in place of reading every name each round
//   - 2026-11-08 10:35: takes _Counts, and a nil one is no next round (C108, B219, B220)
//   - 2026-11-10 00:25: renamed from _Next, as a test in tests/ needs it (C120a, Q314)
func Next(
	round []string,
	waits *Counts,
	waiters map[string][]string,
) []string {
	var next []string

	for _, name := range round {
		for _, waiter := range waiters[name] {
			if waits._Release(waiter) {
				next = append(
					next,
					waiter,
				)
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
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
func _Bound(
	name string,
	held *workflowpb.Constant,
) (syntax.Expr, error) {
	if name == "" || held == nil {
		return nil, fmt.Errorf(
			"constant: %w",
			ErrForm,
		)
	}

	switch kind := held.GetKind().(type) {
	case *workflowpb.Constant_Value:
		return _Value(kind.Value)

	case *workflowpb.Constant_Call:
		if kind.Call.GetResult() != "" {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrForm,
			)
		}

		if _Both(kind.Call) {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrForm,
			)
		}

		return _CallExpr(kind.Call)

	default:
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(name),
			ErrForm,
		)
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
			return nil, fmt.Errorf(
				"arg: %w",
				ErrForm,
			)
		}

		call, err := _Declaration(flow.GetArgs()[name])
		if err != nil {
			return nil, err
		}

		stmts = append(
			stmts,
			_Emit(
				name,
				call,
			),
		)
	}

	return stmts, nil
}
