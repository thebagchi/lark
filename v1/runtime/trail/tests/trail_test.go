package tests

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestEnter_Cycle proves a container already on the path is the cycle error,
// that the error is an ordinary one, and that its text is the starlark-style
// sentence of the decision.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func TestEnter_Cycle(t *testing.T) {
	var (
		path = trail.New(context.Background())
		list = starlark.NewList(nil)
		err  = trail.Enter(
			path,
			list,
		)
	)
	if err != nil {
		t.Fatalf(
			"first Enter: %v",
			err,
		)
	}

	err = trail.Enter(
		path,
		list,
	)
	if !errors.Is(
		err,
		trail.ErrCyclic,
	) {
		t.Fatalf(
			"second Enter = %v, want ErrCyclic",
			err,
		)
	}

	if errors.Is(
		err,
		trail.ErrFatal,
	) {
		t.Fatalf(
			"a cycle must not end the run: %v",
			err,
		)
	}

	if err.Error() != "cycle in value" {
		t.Fatalf(
			"text = %q, want %q",
			err.Error(),
			"cycle in value",
		)
	}
}

// TestEnter_SharedChildIsNotACycle proves a container reached by two routes is
// not a cycle, because Leave takes it off the path.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnter_SharedChildIsNotACycle(t *testing.T) {
	var (
		path   = trail.New(context.Background())
		parent = starlark.NewList(nil)
		shared = starlark.NewList(nil)
	)

	err := trail.Enter(
		path,
		parent,
	)
	if err != nil {
		t.Fatalf(
			"Enter parent: %v",
			err,
		)
	}

	for round := range 2 {
		err = trail.Enter(
			path,
			shared,
		)
		if err != nil {
			t.Fatalf(
				"Enter shared, round %d: %v",
				round,
				err,
			)
		}

		trail.Leave(path)
	}
}

// TestEnter_DepthLimit proves DEPTH nested containers pass and one more is
// ErrDeep, which wraps ErrFatal and reads as one sentence.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic and ErrDeep, formerly ErrSelfContaining and ErrTooDeep
//     (C121, Q349)
func TestEnter_DepthLimit(t *testing.T) {
	tests := []struct {
		name string
		make func() *common.Trail
	}{
		{
			name: "New",
			make: func() *common.Trail {
				return trail.New(context.Background())
			},
		},
		{
			name: "Bounded",
			make: func() *common.Trail {
				return trail.Bounded(context.Background())
			},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				path := tc.make()

				for level := range common.DEPTH {
					err := trail.Enter(
						path,
						starlark.NewList(nil),
					)
					if err != nil {
						t.Fatalf(
							"level %d: %v",
							level,
							err,
						)
					}
				}

				err := trail.Enter(
					path,
					starlark.NewList(nil),
				)
				if !errors.Is(
					err,
					trail.ErrDeep,
				) {
					t.Fatalf(
						"level %d = %v, want ErrDeep",
						common.DEPTH+1,
						err,
					)
				}

				if !errors.Is(
					err,
					trail.ErrFatal,
				) {
					t.Fatalf(
						"ErrDeep must wrap ErrFatal: %v",
						err,
					)
				}

				if err.Error() != "value nested too deep" {
					t.Fatalf(
						"text = %q",
						err.Error(),
					)
				}

				if errors.Is(
					err,
					trail.ErrCyclic,
				) {
					t.Fatal("too deep must not read as a cycle")
				}
			},
		)
	}
}

// TestBounded_LeavesCyclesToTheWalk proves a Bounded trail accepts a container
// it has already entered, because the walk keeps its own set.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBounded_LeavesCyclesToTheWalk(t *testing.T) {
	var (
		path = trail.Bounded(context.Background())
		list = starlark.NewList(nil)
	)

	for round := range 3 {
		err := trail.Enter(
			path,
			list,
		)
		if err != nil {
			t.Fatalf(
				"round %d: %v",
				round,
				err,
			)
		}
	}
}

// TestEnter_TupleIsNilAndNeverACycle proves a nil container counts for depth
// and is never compared.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnter_TupleIsNilAndNeverACycle(t *testing.T) {
	path := trail.New(context.Background())

	for round := range 3 {
		err := trail.Enter(
			path,
			nil,
		)
		if err != nil {
			t.Fatalf(
				"round %d: %v",
				round,
				err,
			)
		}
	}

	if path.Depth != 3 {
		t.Fatalf(
			"Depth = %d, want 3",
			path.Depth,
		)
	}
}

// TestLeave_WithNothingEnteredDoesNothing proves an unbalanced Leave cannot
// take the depth below zero.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLeave_WithNothingEnteredDoesNothing(t *testing.T) {
	path := trail.New(context.Background())

	trail.Leave(path)

	if path.Depth != 0 {
		t.Fatalf(
			"Depth = %d, want 0",
			path.Depth,
		)
	}
}

// TestTick_LooksAtTheContextEveryPoll proves a cancelled context stops a walk
// within POLL elements and not before the first POLL, and that a context that
// can never be cancelled costs the walk nothing.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestTick_LooksAtTheContextEveryPoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	path := trail.Bounded(ctx)

	var (
		seen int
		err  error
	)

	for seen < 10*common.POLL && err == nil {
		err = trail.Tick(path)
		seen++
	}

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"err = %v, want context.Canceled",
			err,
		)
	}

	if seen != common.POLL {
		t.Fatalf(
			"stopped after %d elements, want %d",
			seen,
			common.POLL,
		)
	}

	quiet := trail.Bounded(context.Background())

	for range 10 * common.POLL {
		err = trail.Tick(quiet)
		if err != nil {
			t.Fatalf(
				"Background: %v",
				err,
			)
		}
	}
}

// TestEnter_CountsContainersAsElements proves a walk made only of containers
// is stopped by a cancel too.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEnter_CountsContainersAsElements(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	path := trail.Bounded(ctx)

	var err error

	for range common.POLL {
		err = trail.Enter(
			path,
			nil,
		)
		trail.Leave(path)
	}

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"err = %v, want context.Canceled",
			err,
		)
	}
}
