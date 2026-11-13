package tests

// This file tests the guards of the reading methods, Scope, Visible, the placing
// helpers and the constant order against a nil, a cycle or a shared value that
// no exported function passes (C108, Q206, Q207; breaker findings graph-F21 to
// F29, F51 to F55, F58, F59; bugs B186 to B194, B214 to B220).
//
// Revisions:
//   - 2026-11-10 00:25: moved to tests/ from breaker_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/internal/declared"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestBreaker_NilInputIsRefusedNotPanicked checks each reader, writer and
// helper refuses a nil node, a nil scope, a typed nil or an index out of range,
// and does not panic. A row passes when its call returns true.
//
// Revisions:
//   - 2026-11-06 22:02: initial creation (C100c, B147 to B155, B179 to B181)
//   - 2026-11-08 10:35: initial creation (C108, B186, B189 to B192, B214 to B218)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-08 22:48: takes the rows of TestGuards_NilInputIsRefusedNotPanicked (C65a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 16:04: Spelled moved to internal/declared (C118 fix round 1)
func TestBreaker_NilInputIsRefusedNotPanicked(t *testing.T) {
	var (
		reading = new(graph.Reading)
		fn      = &workflowpb.Function{
			Name: "test",
			Code: &workflowpb.Function_Body{
				Body: "print('hello')",
			},
		}
		ident = &syntax.Ident{
			Name: "foo",
		}
		nilBinary  *syntax.BinaryExpr
		nilLiteral *syntax.Literal
	)

	tests := []struct {
		name string
		run  func() bool
	}{
		{
			name: "B186 Sleep of a nil call",
			run: func() bool {
				_, ok := reading.Sleep(nil)

				return !ok
			},
		},
		{
			name: "B189 Cancel of a nil call",
			run: func() bool {
				_, ok := reading.Cancel(
					nil,
					new(graph.Scope),
				)

				return !ok
			},
		},
		{
			name: "B190 Invoked with a nil call",
			run: func() bool {
				_, ok := reading.Invoked(
					nil,
					"b",
					nil,
				)

				return !ok
			},
		},
		{
			name: "B191 Code with a nil program",
			run: func() bool {
				_, err := graph.Code(
					nil,
					fn,
				)

				return errors.Is(
					err,
					graph.ErrForm,
				)
			},
		},
		{
			name: "B192 Literal of a nil literal",
			run: func() bool {
				_, ok := graph.Literal(nil)

				return !ok
			},
		},
		{
			name: "B214 Passed with a typed nil binary",
			run: func() bool {
				return graph.Passed(nilBinary) == syntax.Expr(nilBinary)
			},
		},
		{
			name: "B214 Passed with a nil expression",
			run: func() bool {
				return graph.Passed(nil) == nil
			},
		},
		{
			name: "B215 Spelled of a typed nil literal",
			run: func() bool {
				_, ok := declared.Spelled(nilLiteral)

				return !ok
			},
		},
		{
			name: "B216 First with a head before the file",
			run: func() bool {
				return graph.First(
					nil,
					-1,
					5,
				) == 5
			},
		},
		{
			name: "B216 First with a first past the lines",
			run: func() bool {
				return graph.First(
					[]string{"a", "b"},
					0,
					10,
				) == 10
			},
		},
		{
			name: "B217 Last before the file",
			run: func() bool {
				return graph.Last(
					[]string{"# comment"},
					-1,
					"",
				) == 1
			},
		},
		{
			name: "B187 Sees in a top scope with no reading",
			run: func() bool {
				scope := &graph.Scope{
					Top: true,
				}

				return !scope.Sees("test")
			},
		},
		{
			name: "B187 Sees in a scope with locals and no reading",
			run: func() bool {
				scope := &graph.Scope{
					Locals: map[string]bool{
						"secret": true,
					},
				}

				return !scope.Sees("secret") && !scope.Sees("other")
			},
		},
		{
			name: "B218 Params of a nil def",
			run: func() bool {
				_, ok := graph.Params(nil)

				return !ok
			},
		},
		{
			name: "F4 Tails on a nil program",
			run: func() bool {
				var prog *graph.Program

				return prog.Tails([]syntax.Stmt{&syntax.AssignStmt{}}) == nil
			},
		},
		{
			name: "F5 Inner of a nil scope",
			run: func() bool {
				var scope *graph.Scope

				return scope.Inner() == nil
			},
		},
		{
			name: "F5 Expression with a nil scope",
			run: func() bool {
				_, ok := reading.Expression(
					ident,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F10 Condition with a nil scope",
			run: func() bool {
				_, ok := reading.Condition(
					ident,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F14 Times with a nil scope",
			run: func() bool {
				_, ok := reading.Times(
					ident,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F6 Cases with a nil chain",
			run: func() bool {
				return !reading.Cases(
					&workflowpb.Match{},
					nil,
					&graph.Scope{},
				)
			},
		},
		{
			name: "F7 Target with a nil scope",
			run: func() bool {
				_, ok := reading.Target(
					ident,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F8 Timeout with a nil call",
			run: func() bool {
				_, ok := reading.Timeout(
					nil,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F9 If with a nil statement",
			run: func() bool {
				_, ok := reading.If(
					nil,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F13 Loop with a nil statement",
			run: func() bool {
				_, ok := reading.Loop(
					nil,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F18 Assigned with a nil statement",
			run: func() bool {
				_, ok := reading.Assigned(
					nil,
					nil,
				)

				return !ok
			},
		},
		{
			name: "F11 Constants of a nil tree",
			run: func() bool {
				return reading.Constants(
					nil,
					"",
					graph.NewLines(""),
					nil,
				) == nil
			},
		},
		{
			name: "F19 MarkIf of a nil if",
			run: func() bool {
				return graph.MarkIf(
					graph.NewLines(""),
					nil,
					nil,
				) == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if !tt.run() {
					t.Fatal("the call did not refuse the input")
				}
			},
		)
	}
}

// TestBreaker_ScopeCycleEnds checks a scope chain that loops back on itself
// ends the walk: Sees and Spawned answer, they do not hang. The scopes have
// no reading, as in the original reproducer (graph-F22).
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B187)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_ScopeCycleEnds(t *testing.T) {
	var (
		first  = new(graph.Scope)
		second = &graph.Scope{
			Outer: first,
		}
	)

	first.Outer = second

	asked := map[string]func() bool{
		"Sees": func() bool {
			return first.Sees("test")
		},
		"Spawned": func() bool {
			return first.Spawned("test")
		},
	}

	for name, ask := range asked {
		t.Run(
			name,
			func(t *testing.T) {
				done := make(
					chan bool,
					1,
				)

				go func() {
					done <- ask()
				}()

				select {
				case got := <-done:
					if got {
						t.Fatal("a name no scope holds was seen")
					}

				case <-time.After(5 * time.Second):
					t.Fatal("the walk of a cyclic chain did not end")
				}
			},
		)
	}
}

// TestBreaker_ScopeWritesDoNotRace checks one goroutine may write a scope's
// parameters under its guard while another reads them, with -race.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B188, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_ScopeWritesDoNotRace(t *testing.T) {
	scope := &graph.Scope{
		Reading: new(graph.Reading),
		Params: map[string]bool{
			"a": true,
		},
		Locals: map[string]bool{},
		Values: map[string]bool{},
		Spawns: map[string]bool{},
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		for range 1000 {
			if !scope.Sees("a") {
				t.Error("a parameter was not seen")
			}
		}
	}()

	go func() {
		defer wg.Done()

		for range 1000 {
			scope.Guard.Lock()
			scope.Params["b"] = true
			scope.Guard.Unlock()
		}
	}()

	wg.Wait()
}

// TestBreaker_VisibleSpawn checks a spawn on a visible set with no spawn map
// is recorded, and that concurrent spawns of one binding record it once.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B193, B194, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrFork, formerly ErrNotForked (C121, Q348)
func TestBreaker_VisibleSpawn(t *testing.T) {
	spawn := &workflowpb.Spawn{
		Binding: "test-binding",
	}

	t.Run(
		"B193 a nil spawn map",
		func(t *testing.T) {
			var seen = new(graph.Visible)

			err := seen.Spawn(spawn)
			if err != nil {
				t.Fatalf(
					"got %v, want nil",
					err,
				)
			}

			if !seen.Spawned("test-binding") {
				t.Fatal("the binding was not recorded")
			}
		},
	)

	t.Run(
		"B194 ten goroutines, one binding",
		func(t *testing.T) {
			var (
				seen = &graph.Visible{
					Spawns: make(map[string]bool),
				}
				wg      sync.WaitGroup
				mu      sync.Mutex
				allowed int
			)

			for range 10 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					err := seen.Spawn(spawn)
					if err == nil {
						mu.Lock()
						allowed++
						mu.Unlock()

						return
					}

					if !errors.Is(
						err,
						graph.ErrFork,
					) {
						t.Errorf(
							"got %v, want nil or ErrFork",
							err,
						)
					}
				}()
			}

			wg.Wait()

			if allowed != 1 {
				t.Fatalf(
					"%d spawns of one binding were allowed, want 1",
					allowed,
				)
			}
		},
	)
}

// TestBreaker_NextOrder checks the round after a round, with no counts, and
// when two goroutines share one set of counts, with -race.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B219, B220, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_NextOrder(t *testing.T) {
	waiters := map[string][]string{
		"a": {"b"},
	}

	t.Run(
		"B219 nil counts",
		func(t *testing.T) {
			got := graph.Next(
				[]string{"a"},
				nil,
				waiters,
			)
			if len(got) != 0 {
				t.Fatalf(
					"got %v, want no next round",
					got,
				)
			}
		},
	)

	t.Run(
		"B219 counts with no map",
		func(t *testing.T) {
			got := graph.Next(
				[]string{"a"},
				new(graph.Counts),
				waiters,
			)
			if len(got) != 0 {
				t.Fatalf(
					"got %v, want no next round",
					got,
				)
			}
		},
	)

	t.Run(
		"B220 two goroutines, one set of counts",
		func(t *testing.T) {
			var (
				counts = &graph.Counts{
					Held: map[string]int{
						"b": 1,
					},
				}
				wg    sync.WaitGroup
				mu    sync.Mutex
				ready [][]string
			)

			for range 2 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					for range 100 {
						next := graph.Next(
							[]string{"a"},
							counts,
							waiters,
						)
						if len(next) == 0 {
							continue
						}

						mu.Lock()

						ready = append(
							ready,
							next,
						)
						mu.Unlock()
					}
				}()
			}

			wg.Wait()

			if len(ready) != 1 || !slices.Equal(
				ready[0],
				[]string{"b"},
			) {
				t.Fatalf(
					"got %v, want b ready once",
					ready,
				)
			}
		},
	)
}

// TestBreaker_VisibleBindNilMap checks a result bound on a visible set with no
// values map is recorded, and is no panic (C108 fix round 1, B223, Q206, Q65).
//
// Revisions:
//   - 2026-11-08 10:51: initial creation (C108, B223)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_VisibleBindNilMap(t *testing.T) {
	seen := new(graph.Visible)

	seen.Bind("result")

	if !seen.Holds(
		"result",
		true,
	) {
		t.Fatal("the result was not recorded")
	}
}

// TestBreaker_ScopeBindingsDoNotRace checks one goroutine may bind a result or
// a spawn on a scope while another asks for it, with -race. Each row needs the
// lock of its writer and the lock of its reader (C108 fix round 2, B188, Q207).
//
// Revisions:
//   - 2026-11-08 12:46: initial creation (C108, B188)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_ScopeBindingsDoNotRace(t *testing.T) {
	tests := []struct {
		name string
		bind func(
			*graph.Scope,
			string,
		)
		ask func(
			*graph.Scope,
			string,
		) bool
	}{
		{
			name: "results",
			bind: (*graph.Scope).Bind,
			ask:  (*graph.Scope).Sees,
		},
		{
			name: "results read by _Binds",
			bind: (*graph.Scope).Bind,
			ask:  (*graph.Scope).Binds,
		},
		{
			name: "spawns",
			bind: func(
				scope *graph.Scope,
				name string,
			) {
				scope.Fork(name)
			},
			ask: (*graph.Scope).Spawned,
		},
		{
			name: "spawns read by _Forks",
			bind: func(
				scope *graph.Scope,
				name string,
			) {
				scope.Fork(name)
			},
			ask: (*graph.Scope).Forks,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					scope = &graph.Scope{
						Reading: new(graph.Reading),
						Params:  map[string]bool{},
						Locals:  map[string]bool{},
						Values:  map[string]bool{},
						Spawns:  map[string]bool{},
					}
					wg sync.WaitGroup
				)

				wg.Add(2)

				go func() {
					defer wg.Done()

					for range 1000 {
						tt.bind(
							scope,
							"name",
						)
					}
				}()

				go func() {
					defer wg.Done()

					for range 1000 {
						tt.ask(
							scope,
							"name",
						)
					}
				}()

				wg.Wait()

				if !tt.ask(
					scope,
					"name",
				) {
					t.Fatal("the binding was lost")
				}
			},
		)
	}
}

// TestBreaker_NilReadingIsRefused checks every method of a nil reading, and
// every scope method that reaches one, refuses or reads nothing, and does not
// panic. A row passes when its call returns true (C108 fix round 3, B187, Q65,
// Q206).
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B187)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_NilReadingIsRefused(t *testing.T) {
	var (
		reading *graph.Reading
		scope   = new(graph.Scope)
		def     = &syntax.DefStmt{
			Name: &syntax.Ident{
				Name: "f",
			},
		}
		tree = new(syntax.File)
		call = &syntax.CallExpr{
			Fn: &syntax.Ident{
				Name: "f",
			},
		}
		refused = func(err error) bool {
			return errors.Is(
				err,
				graph.ErrForm,
			)
		}
	)

	tests := []struct {
		name string
		run  func() bool
	}{
		{
			name: "Listed",
			run: func() bool {
				return !reading.Listed("x")
			},
		},
		{
			name: "Builtin",
			run: func() bool {
				return !reading.Builtin("x")
			},
		},
		{
			name: "Callable through a scope with no reading",
			run: func() bool {
				return !scope.Callable("x")
			},
		},
		{
			name: "Usable through a scope with no reading",
			run: func() bool {
				return !scope.Usable("x")
			},
		},
		{
			name: "Call through a scope with no reading",
			run: func() bool {
				_, ok := reading.Call(
					call,
					scope,
				)

				return !ok
			},
		},
		{
			name: "Runs",
			run: func() bool {
				return !reading.Runs(
					new(syntax.AssignStmt),
					"",
				)
			},
		},
		{
			name: "Placed",
			run: func() bool {
				return refused(
					reading.Placed(
						&syntax.ExprStmt{
							X: call,
						},
						"",
						nil,
					),
				)
			},
		},
		{
			name: "Flow",
			run: func() bool {
				return reading.Flow() == nil
			},
		},
		{
			name: "Function",
			run: func() bool {
				return reading.Function(def) == nil
			},
		},
		{
			name: "Spine",
			run: func() bool {
				reading.Spine(nil)

				return true
			},
		},
		{
			name: "Module",
			run: func() bool {
				_, err := reading.Module(
					"",
					"",
				)

				return refused(err)
			},
		},
		{
			name: "Read",
			run: func() bool {
				return refused(
					reading.Read(
						tree,
						"",
						"",
					),
				)
			},
		},
		{
			name: "Whole",
			run: func() bool {
				return refused(
					reading.Whole(
						tree,
						"",
					),
				)
			},
		},
		{
			name: "Admitted",
			run: func() bool {
				return refused(reading.Admitted())
			},
		},
		{
			name: "Shadows",
			run: func() bool {
				return refused(reading.Shadows())
			},
		},
		{
			name: "Declare",
			run: func() bool {
				return refused(
					reading.Declare(
						def,
						"",
						nil,
					),
				)
			},
		},
		{
			name: "Loaded",
			run: func() bool {
				return refused(
					reading.Loaded(
						"",
						nil,
					),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if !tt.run() {
					t.Fatal("the call did not refuse the nil reading")
				}
			},
		)
	}
}

// TestBreaker_InnerScopeReadsUnderTheOwnersGuard checks a list inside a
// statement, which shares params with the list around it, reads them under the
// guard of the scope that owns them, so a writer that takes that guard does not
// race the reader, for a scope and for a visible set (C108 fix round 3, B188,
// B194, Q207).
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B188, B194)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBreaker_InnerScopeReadsUnderTheOwnersGuard(t *testing.T) {
	var (
		outer = &graph.Scope{
			Reading: new(graph.Reading),
			Params: map[string]bool{
				"a": true,
			},
			Locals: map[string]bool{},
			Values: map[string]bool{},
			Spawns: map[string]bool{},
		}
		visible = &graph.Visible{
			Params: map[string]bool{
				"a": true,
			},
			Values: map[string]bool{},
			Spawns: map[string]bool{},
		}
		inner  = outer.Inner()
		within = visible.Inner()
		wg     sync.WaitGroup
	)

	wg.Add(4)

	go func() {
		defer wg.Done()

		for range 1000 {
			outer.Guard.Lock()
			outer.Params["b"] = true
			outer.Guard.Unlock()
		}
	}()

	go func() {
		defer wg.Done()

		for range 1000 {
			if !inner.Sees("a") {
				t.Error("an inner scope lost a parameter")
			}
		}
	}()

	go func() {
		defer wg.Done()

		for range 1000 {
			visible.Guard.Lock()
			visible.Params["b"] = true
			visible.Guard.Unlock()
		}
	}()

	go func() {
		defer wg.Done()

		for range 1000 {
			if !within.Sees("a") {
				t.Error("an inner set lost a parameter")
			}
		}
	}()

	wg.Wait()
}
