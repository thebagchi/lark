package tests

// This file tests the guards of the name walk, the copier, the load copiers,
// the marker and the script finders against a nil, a typed nil, a shared value
// or a load with more names than loads. Compile and Copied reach most of them
// only through a contained panic, and no exported function hands them a shared
// value, so the tests call them directly (C108, Q206, Q207; breaker findings
// graph-F31, F33 to F50; bugs B195, B197 to B213).
//
// Revisions:
//   - 2026-11-10 01:37: moved to tests/ from breaker_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"errors"
	"sync"
	"testing"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/dialect/internal/rewrite"
)

// TestBreaker_NilInputIsRefusedNotPanicked checks each unexported helper
// refuses a nil node, a nil map or a list that does not match, and does not
// panic. A row passes when its call returns true.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B195, B197, B199 to B209, B211 to B213)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBreaker_NilInputIsRefusedNotPanicked(t *testing.T) {
	var (
		first = &syntax.Ident{
			Name: "x",
		}
		second = &syntax.Ident{
			Name: "y",
		}
	)

	tests := []struct {
		name string
		run  func() bool
	}{
		{
			name: "B195 Targets into a nil map",
			run: func() bool {
				rewrite.Targets(
					&syntax.Ident{
						Name: "foo",
					},
					nil,
				)

				return true
			},
		},
		{
			name: "B197 If of a nil clause",
			run: func() bool {
				copier := new(rewrite.Copier)

				return copier.If(nil) == nil && errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				)
			},
		},
		{
			name: "B199 Taken of a nil file",
			run: func() bool {
				return len(rewrite.Taken(nil)) == 0
			},
		},
		{
			name: "B200 Stmts of a nil copier",
			run: func() bool {
				var copier *rewrite.Copier

				return copier.Stmts([]syntax.Stmt{nil}) == nil
			},
		},
		{
			name: "B200 Stmts with a nil statement",
			run: func() bool {
				copier := new(rewrite.Copier)

				copier.Stmts([]syntax.Stmt{nil})

				return errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				)
			},
		},
		{
			name: "B201 For of a nil clause",
			run: func() bool {
				copier := new(rewrite.Copier)

				return copier.For(nil) == nil && errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				)
			},
		},
		{
			name: "B202 Load of a nil load",
			run: func() bool {
				return rewrite.Load(nil) == nil
			},
		},
		{
			name: "B203 Load with no module",
			run: func() bool {
				fresh := rewrite.Load(&syntax.LoadStmt{})

				return fresh != nil && fresh.Module == nil
			},
		},
		{
			name: "B204 Idents with a nil name",
			run: func() bool {
				fresh := rewrite.Idents([]*syntax.Ident{nil})

				return len(fresh) == 1 && fresh[0] == nil
			},
		},
		{
			name: "B205 Loaded of a nil load",
			run: func() bool {
				return rewrite.Loaded(
					nil,
					nil,
				) == nil
			},
		},
		{
			name: "B205 Loaded with a shorter From",
			run: func() bool {
				var (
					load = &syntax.LoadStmt{
						To:   []*syntax.Ident{first, second},
						From: []*syntax.Ident{first},
					}
					fresh = rewrite.Loaded(
						load,
						[]*syntax.Ident{first},
					)
				)

				return len(fresh) == 2 && fresh[0] == first &&
					fresh[1] != nil && fresh[1] != second
			},
		},
		{
			name: "B205 Loaded with a longer from",
			run: func() bool {
				var (
					load = &syntax.LoadStmt{
						To:   []*syntax.Ident{first, second},
						From: []*syntax.Ident{first},
					}
					fresh = rewrite.Loaded(
						load,
						[]*syntax.Ident{first, second},
					)
				)

				return len(fresh) == 2 && fresh[0] == first &&
					fresh[1] != nil && fresh[1] != second
			},
		},
		{
			name: "B206 Assign of a nil statement",
			run: func() bool {
				new(rewrite.Marker).Assign(
					nil,
					"",
				)

				return true
			},
		},
		{
			name: "B207 Lambdas of a nil file",
			run: func() bool {
				new(rewrite.Marker).Lambdas(nil)

				return true
			},
		},
		{
			name: "B208 For of a nil call",
			run: func() bool {
				marker := &rewrite.Marker{
					Marks: make(map[rewrite.At]*rewrite.Mark),
				}

				return marker.For(nil) != nil && len(marker.Marks) == 0
			},
		},
		{
			name: "B209 For with no marks map",
			run: func() bool {
				var (
					marker = new(rewrite.Marker)
					mark   = marker.For(&syntax.CallExpr{})
				)

				return mark != nil && len(marker.Marks) == 1
			},
		},
		{
			name: "B211 Scripts of a nil file",
			run: func() bool {
				return len(rewrite.Scripts(nil)) == 0
			},
		},
		{
			name: "B212 Scripts with a nil def name",
			run: func() bool {
				found := rewrite.Scripts(
					&syntax.File{
						Stmts: []syntax.Stmt{
							&syntax.DefStmt{},
						},
					},
				)

				return len(found) == 0
			},
		},
		{
			name: "B212 Scripts with a nil load and a nil name",
			run: func() bool {
				var nilLoad *syntax.LoadStmt

				found := rewrite.Scripts(
					&syntax.File{
						Stmts: []syntax.Stmt{
							nilLoad,
							&syntax.LoadStmt{
								To: []*syntax.Ident{nil, first},
							},
						},
					},
				)

				return len(found) == 1 && found[first]
			},
		},
		{
			name: "B213 Handed of a nil call",
			run: func() bool {
				return rewrite.Handed(nil) == nil
			},
		},
		{
			name: "F42 Marks of a nil file",
			run: func() bool {
				return len(rewrite.Marks(nil)) == 0
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

// TestBreaker_SharedStateDoesNotRace checks one copier and one mark may be
// used from many goroutines, with -race, and that no update is lost.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B198, B210, Q207)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBreaker_SharedStateDoesNotRace(t *testing.T) {
	t.Run(
		"B198 a hundred goroutines refuse",
		func(t *testing.T) {
			var (
				copier = new(rewrite.Copier)
				wg     sync.WaitGroup
			)

			for range 100 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					copier.Refuse(nil)
				}()
			}

			wg.Wait()

			if !errors.Is(
				copier.Err,
				dialect.ErrUncopied,
			) {
				t.Fatalf(
					"got %v, want ErrUncopied",
					copier.Err,
				)
			}
		},
	)

	t.Run(
		"B198 refusals and reads of the first refusal",
		func(t *testing.T) {
			var (
				copier = new(rewrite.Copier)
				wg     sync.WaitGroup
			)

			for range 50 {
				wg.Add(2)

				go func() {
					defer wg.Done()

					copier.Refuse(nil)
				}()

				go func() {
					defer wg.Done()

					_ = copier.Failed()
				}()
			}

			wg.Wait()

			if copier.Failed() == nil {
				t.Fatal("no refusal was kept")
			}
		},
	)

	t.Run(
		"B210 reports and changes of one mark",
		func(t *testing.T) {
			var (
				mark = new(rewrite.Mark)
				call = &syntax.CallExpr{
					Fn: &syntax.Ident{
						Name: "f",
					},
				}
				wg sync.WaitGroup
			)

			for range 50 {
				wg.Add(2)

				go func() {
					defer wg.Done()

					mark.Report()
				}()

				go func() {
					defer wg.Done()

					mark.Change(call)
				}()
			}

			wg.Wait()
		},
	)

	t.Run(
		"B210 ten goroutines change one call",
		func(t *testing.T) {
			var (
				mark = &rewrite.Mark{
					Reported: true,
				}
				call = &syntax.CallExpr{
					Fn: &syntax.Ident{
						Name: "f",
					},
				}
				wg sync.WaitGroup
			)

			for range 10 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					mark.Change(call)
				}()
			}

			wg.Wait()

			if len(call.Args) != 10 {
				t.Fatalf(
					"got %d arguments, want 10: an update was lost",
					len(call.Args),
				)
			}
		},
	)
}

// TestBreaker_TypedNilNodeIsRefused checks the copier refuses a typed nil of
// every statement and expression kind at one seam, keeps the first refusal as
// ErrUncopied, and does not panic (C108 fix round 1, B225, Q206, Q65).
//
// Revisions:
//   - 2026-11-08 10:51: initial creation (C108, B225)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBreaker_TypedNilNodeIsRefused(t *testing.T) {
	stmts := map[string]syntax.Stmt{
		"assign": (*syntax.AssignStmt)(nil),
		"branch": (*syntax.BranchStmt)(nil),
		"def":    (*syntax.DefStmt)(nil),
		"expr":   (*syntax.ExprStmt)(nil),
		"for":    (*syntax.ForStmt)(nil),
		"if":     (*syntax.IfStmt)(nil),
		"load":   (*syntax.LoadStmt)(nil),
		"return": (*syntax.ReturnStmt)(nil),
		"while":  (*syntax.WhileStmt)(nil),
	}

	for name, stmt := range stmts {
		t.Run(
			"statement "+name,
			func(t *testing.T) {
				copier := new(rewrite.Copier)

				copier.Stmt(stmt)

				if !errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				) {
					t.Fatalf(
						"got %v, want ErrUncopied",
						copier.Err,
					)
				}
			},
		)
	}

	exprs := map[string]syntax.Expr{
		"ident":         (*syntax.Ident)(nil),
		"literal":       (*syntax.Literal)(nil),
		"paren":         (*syntax.ParenExpr)(nil),
		"unary":         (*syntax.UnaryExpr)(nil),
		"binary":        (*syntax.BinaryExpr)(nil),
		"cond":          (*syntax.CondExpr)(nil),
		"dot":           (*syntax.DotExpr)(nil),
		"index":         (*syntax.IndexExpr)(nil),
		"slice":         (*syntax.SliceExpr)(nil),
		"call":          (*syntax.CallExpr)(nil),
		"lambda":        (*syntax.LambdaExpr)(nil),
		"list":          (*syntax.ListExpr)(nil),
		"tuple":         (*syntax.TupleExpr)(nil),
		"dict":          (*syntax.DictExpr)(nil),
		"entry":         (*syntax.DictEntry)(nil),
		"comprehension": (*syntax.Comprehension)(nil),
	}

	for name, expr := range exprs {
		t.Run(
			"expression "+name,
			func(t *testing.T) {
				copier := new(rewrite.Copier)

				copier.Expr(expr)

				if !errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				) {
					t.Fatalf(
						"got %v, want ErrUncopied",
						copier.Err,
					)
				}
			},
		)
	}

	t.Run(
		"a nil name in a def and a dot",
		func(t *testing.T) {
			for _, stmt := range []syntax.Stmt{
				&syntax.DefStmt{},
				&syntax.ExprStmt{
					X: &syntax.DotExpr{
						X: &syntax.Ident{
							Name: "x",
						},
					},
				},
			} {
				copier := new(rewrite.Copier)

				copier.Stmt(stmt)

				if !errors.Is(
					copier.Err,
					dialect.ErrUncopied,
				) {
					t.Fatalf(
						"got %v, want ErrUncopied",
						copier.Err,
					)
				}
			}
		},
	)

	t.Run(
		"a load with a nil name",
		func(t *testing.T) {
			copier := new(rewrite.Copier)

			copier.Stmt(
				&syntax.LoadStmt{
					Module: &syntax.Literal{},
					From:   []*syntax.Ident{nil},
					To:     []*syntax.Ident{nil},
				},
			)

			if !errors.Is(
				copier.Err,
				dialect.ErrUncopied,
			) {
				t.Fatalf(
					"got %v, want ErrUncopied",
					copier.Err,
				)
			}
		},
	)
}

// TestBreaker_LoadWithMoreNamesThanLoadsIsUncopied checks Copied refuses a load
// that binds more names than it loads, as it did when the copy panicked on it,
// since the doc of Copied says a node with a part missing is ErrUncopied (C108
// fix round 2, B205).
//
// Revisions:
//   - 2026-11-08 12:46: initial creation (C108, B205)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBreaker_LoadWithMoreNamesThanLoadsIsUncopied(t *testing.T) {
	var (
		first = &syntax.Ident{
			Name: "x",
		}
		second = &syntax.Ident{
			Name: "y",
		}
		load = &syntax.LoadStmt{
			Module: &syntax.Literal{},
			To:     []*syntax.Ident{first, second},
			From:   []*syntax.Ident{first},
		}
	)

	_, err := dialect.Copied(
		&syntax.File{
			Stmts: []syntax.Stmt{load},
		},
	)
	if !errors.Is(
		err,
		dialect.ErrUncopied,
	) {
		t.Fatalf(
			"got %v, want ErrUncopied",
			err,
		)
	}
}
