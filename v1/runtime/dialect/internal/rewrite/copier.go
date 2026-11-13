package rewrite

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
)

// ErrUncopied is a syntax node this dialect has no copy for.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
var (
	ErrUncopied = errors.New("a syntax node the dialect cannot copy")
)

// Copier copies a parsed tree, node for node, before it is resolved.
//
// Each node is copied whole and then each child is replaced by its own copy,
// so a field the parser adds is kept, and no node is shared. err is the first
// node met that no case names; the copy goes on, and Compile refuses it. guard
// holds every read and write of err, so goroutines that share a copier keep
// the first node met and do not race.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 10:35: the guard field makes writes to err safe (C108, B198, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Copier to Copier; renamed
//     the fields err to Err, for the tests in tests/ (C120b, Q315, Q314)
type Copier struct {
	guard sync.Mutex
	Err   error
}

// Stmts copies a list of statements, keeping a nil list nil, since an if with
// no else is told apart by its nil False.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil copier copies nothing (C108, B200)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Stmts to Stmts, for the
//     tests in tests/ (C120b, Q315, Q314)
func (c *Copier) Stmts(stmts []syntax.Stmt) []syntax.Stmt {
	if c == nil || stmts == nil {
		return nil
	}

	fresh := make(
		[]syntax.Stmt,
		len(stmts),
	)

	for idx, stmt := range stmts {
		fresh[idx] = c.Stmt(stmt)
	}

	return fresh
}

// Stmt copies one statement. A nil statement, or an interface that holds a nil
// pointer of any statement kind, is refused here, once, in place of in each
// case.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: copies a load through _Imported, and refuses a typed
//     nil statement at this one seam (C108, B202, B203, B225)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Stmt to Stmt, for the tests
//     in tests/ (C120b, Q315, Q314)
//   - 2026-11-11 00:40: finds a nil statement with nilcheck.Absent (C122, B268)
func (c *Copier) Stmt(stmt syntax.Stmt) syntax.Stmt {
	if nilcheck.Absent(stmt) {
		c.Refuse(stmt)

		return stmt
	}

	return c._Whole(stmt)
}

// Expr copies one expression, or keeps nil for an optional part left out.
//
// A name, a literal and an operator are here. The rest pass to _Postfix and
// then to _Composite, which keeps each switch one screen long.
//
// A typed nil of any expression kind is refused here, once, in place of in
// each case.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:54: refuses a typed nil expression at this one seam (C108, B225)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Expr to Expr, for the tests
//     in tests/ (C120b, Q315, Q314)
//   - 2026-11-11 00:40: finds a nil expression with nilcheck.Absent (C122, B268)
func (c *Copier) Expr(expr syntax.Expr) syntax.Expr {
	if expr == nil {
		return nil
	}

	if nilcheck.Absent(expr) {
		c.Refuse(expr)

		return expr
	}

	switch expr := expr.(type) {
	case *syntax.Ident:
		return _Ident(expr)

	case *syntax.Literal:
		fresh := *expr

		return &fresh

	case *syntax.ParenExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)

		return &fresh

	case *syntax.UnaryExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)

		return &fresh

	case *syntax.BinaryExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)
		fresh.Y = c.Expr(expr.Y)

		return &fresh

	case *syntax.CondExpr:
		fresh := *expr
		fresh.Cond = c.Expr(expr.Cond)
		fresh.True = c.Expr(expr.True)
		fresh.False = c.Expr(expr.False)

		return &fresh

	default:
		return c._Postfix(expr)
	}
}

// For copies one for clause.
//
// A nil clause is refused and copies as nil.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil clause is refused (C108, B201)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _For to For, for the tests in
//     tests/ (C120b, Q315, Q314)
func (c *Copier) For(clause *syntax.ForClause) *syntax.ForClause {
	if clause == nil {
		c.Refuse(clause)

		return nil
	}

	fresh := *clause
	fresh.Vars = c.Expr(clause.Vars)
	fresh.X = c.Expr(clause.X)

	return &fresh
}

// If copies one if clause.
//
// A nil clause is refused and copies as nil.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil clause is refused (C108, B197)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _If to If, for the tests in
//     tests/ (C120b, Q315, Q314)
func (c *Copier) If(clause *syntax.IfClause) *syntax.IfClause {
	if clause == nil {
		c.Refuse(clause)

		return nil
	}

	fresh := *clause
	fresh.Cond = c.Expr(clause.Cond)

	return &fresh
}

// Failed is the first node the copy refused, read under the guard.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B198, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Failed to Failed, for the
//     tests in tests/ (C120b, Q315, Q314)
func (c *Copier) Failed() error {
	c.guard.Lock()
	defer c.guard.Unlock()

	return c.Err
}

// Refuse keeps the first node met that no case names.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: keeps the first node under the guard (C108, B198)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Refuse to Refuse, for the
//     tests in tests/ (C120b, Q315, Q314)
func (c *Copier) Refuse(node syntax.Node) {
	c.guard.Lock()
	defer c.guard.Unlock()

	if c.Err != nil {
		return
	}

	c.Err = fmt.Errorf(
		"%w: %T",
		ErrUncopied,
		node,
	)
}

// _Whole copies one statement that is there, node whole.
//
// Revisions:
//   - 2026-11-08 10:54: initial creation, the switch _Stmt had (C108, B225)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Whole(stmt syntax.Stmt) syntax.Stmt {
	switch stmt := stmt.(type) {
	case *syntax.AssignStmt:
		fresh := *stmt
		fresh.LHS = c.Expr(stmt.LHS)
		fresh.RHS = c.Expr(stmt.RHS)

		return &fresh

	case *syntax.BranchStmt:
		fresh := *stmt

		return &fresh

	case *syntax.DefStmt:
		fresh := *stmt
		fresh.Name = c._Name(stmt.Name)
		fresh.Params = c._Exprs(stmt.Params)
		fresh.Body = c.Stmts(stmt.Body)

		return &fresh

	case *syntax.ExprStmt:
		fresh := *stmt
		fresh.X = c.Expr(stmt.X)

		return &fresh

	case *syntax.ForStmt:
		fresh := *stmt
		fresh.Vars = c.Expr(stmt.Vars)
		fresh.X = c.Expr(stmt.X)
		fresh.Body = c.Stmts(stmt.Body)

		return &fresh

	case *syntax.IfStmt:
		fresh := *stmt
		fresh.Cond = c.Expr(stmt.Cond)
		fresh.True = c.Stmts(stmt.True)
		fresh.False = c.Stmts(stmt.False)

		return &fresh

	case *syntax.LoadStmt:
		return c._Imported(stmt)

	case *syntax.ReturnStmt:
		fresh := *stmt
		fresh.Result = c.Expr(stmt.Result)

		return &fresh

	case *syntax.WhileStmt:
		fresh := *stmt
		fresh.Cond = c.Expr(stmt.Cond)
		fresh.Body = c.Stmts(stmt.Body)

		return &fresh

	default:
		c.Refuse(stmt)

		return stmt
	}
}

// _Imported copies a load statement, and refuses one that lacks a part, since
// the copy would hold a part the resolver reads or a name with no counterpart.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B202, B203, B205, B225)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Imported(load *syntax.LoadStmt) syntax.Stmt {
	if _Lacking(load) {
		c.Refuse(load)

		return load
	}

	return Load(load)
}

// _Lacking reports whether load is nil, names no module, holds a nil name, or
// binds more names than it loads.
//
// Revisions:
//   - 2026-11-08 12:46: initial creation (C108, B205)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Lacking(load *syntax.LoadStmt) bool {
	return load == nil || load.Module == nil ||
		len(load.From) < len(load.To) ||
		slices.Contains(
			load.From,
			nil,
		) ||
		slices.Contains(
			load.To,
			nil,
		)
}

// _Exprs copies a list of expressions, keeping a nil list nil.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Exprs(exprs []syntax.Expr) []syntax.Expr {
	if exprs == nil {
		return nil
	}

	fresh := make(
		[]syntax.Expr,
		len(exprs),
	)

	for idx, expr := range exprs {
		fresh[idx] = c.Expr(expr)
	}

	return fresh
}

// _Postfix copies an expression written after the operand it reads: an
// attribute, an index, a slice or a call.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Postfix(expr syntax.Expr) syntax.Expr {
	switch expr := expr.(type) {
	case *syntax.DotExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)
		fresh.Name = c._Name(expr.Name)

		return &fresh

	case *syntax.IndexExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)
		fresh.Y = c.Expr(expr.Y)

		return &fresh

	case *syntax.SliceExpr:
		fresh := *expr
		fresh.X = c.Expr(expr.X)
		fresh.Lo = c.Expr(expr.Lo)
		fresh.Hi = c.Expr(expr.Hi)
		fresh.Step = c.Expr(expr.Step)

		return &fresh

	case *syntax.CallExpr:
		fresh := *expr
		fresh.Fn = c.Expr(expr.Fn)
		fresh.Args = c._Exprs(expr.Args)

		return &fresh

	default:
		return c._Composite(expr)
	}
}

// _Composite copies an expression that holds a list or a body of its own.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Composite(expr syntax.Expr) syntax.Expr {
	switch expr := expr.(type) {
	case *syntax.LambdaExpr:
		fresh := *expr
		fresh.Params = c._Exprs(expr.Params)
		fresh.Body = c.Expr(expr.Body)

		return &fresh

	case *syntax.ListExpr:
		fresh := *expr
		fresh.List = c._Exprs(expr.List)

		return &fresh

	case *syntax.TupleExpr:
		fresh := *expr
		fresh.List = c._Exprs(expr.List)

		return &fresh

	case *syntax.DictExpr:
		fresh := *expr
		fresh.List = c._Exprs(expr.List)

		return &fresh

	case *syntax.DictEntry:
		fresh := *expr
		fresh.Key = c.Expr(expr.Key)
		fresh.Value = c.Expr(expr.Value)

		return &fresh

	case *syntax.Comprehension:
		fresh := *expr
		fresh.Body = c.Expr(expr.Body)
		fresh.Clauses = c._Clauses(expr.Clauses)

		return &fresh

	default:
		c.Refuse(expr)

		return expr
	}
}

// _Clauses copies the for and if clauses of a comprehension.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Clauses(clauses []syntax.Node) []syntax.Node {
	fresh := make(
		[]syntax.Node,
		len(clauses),
	)

	for idx, clause := range clauses {
		switch clause := clause.(type) {
		case *syntax.ForClause:
			fresh[idx] = c.For(clause)

		case *syntax.IfClause:
			fresh[idx] = c.If(clause)

		default:
			c.Refuse(clause)
			fresh[idx] = clause
		}
	}

	return fresh
}

// _Name copies one name, and refuses a nil one, as the copy of a def or an
// attribute with no name would hold a part the resolver reads.
//
// Revisions:
//   - 2026-11-08 10:54: initial creation (C108, B204, B225)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (c *Copier) _Name(ident *syntax.Ident) *syntax.Ident {
	if ident == nil {
		c.Refuse(ident)

		return nil
	}

	return _Ident(ident)
}

// Load copies a load statement: the module it names, the names it loads and
// the names it binds.
//
// A nil load copies as nil, and a load with no module keeps none.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil load copies as nil, and a missing module stays missing
//     (C108, B202, B203)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Load to Load, for the tests
//     in tests/ (C120b, Q315, Q314)
func Load(load *syntax.LoadStmt) *syntax.LoadStmt {
	if load == nil {
		return nil
	}

	fresh := *load

	if load.Module != nil {
		module := *load.Module

		fresh.Module = &module
	}

	fresh.From = Idents(load.From)
	fresh.To = Loaded(
		load,
		fresh.From,
	)

	return &fresh
}

// Idents copies a list of names.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Idents to Idents, for the
//     tests in tests/ (C120b, Q315, Q314)
func Idents(idents []*syntax.Ident) []*syntax.Ident {
	fresh := make(
		[]*syntax.Ident,
		len(idents),
	)

	for idx, ident := range idents {
		fresh[idx] = _Ident(ident)
	}

	return fresh
}

// Loaded copies the names a load binds, given the copies of the names it
// loads.
//
// The parser writes load("m", "x") as one name that is both loaded and bound,
// so where the two were one node, the copy is one node too.
//
// A nil load binds nothing. A name with no counterpart in from or in the
// load's own From is copied alone.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil load binds nothing, and lists of unequal length copy by
//     name (C108, B205)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Loaded to Loaded, for the
//     tests in tests/ (C120b, Q315, Q314)
func Loaded(
	load *syntax.LoadStmt,
	from []*syntax.Ident,
) []*syntax.Ident {
	if load == nil {
		return nil
	}

	fresh := make(
		[]*syntax.Ident,
		len(load.To),
	)

	for idx, to := range load.To {
		if idx < len(from) && idx < len(load.From) && to == load.From[idx] {
			fresh[idx] = from[idx]

			continue
		}

		fresh[idx] = _Ident(to)
	}

	return fresh
}

// _Ident copies one name, the node the resolver writes a binding into.
//
// A nil name copies as nil.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
//   - 2026-11-08 10:35: a nil name copies as nil (C108, B204)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Ident(ident *syntax.Ident) *syntax.Ident {
	if ident == nil {
		return nil
	}

	fresh := *ident

	return &fresh
}
