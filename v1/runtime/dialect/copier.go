package dialect

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"
)

// ErrUncopied is a syntax node this dialect has no copy for.
var (
	ErrUncopied = errors.New("a syntax node the dialect cannot copy")
)

// _Copier copies a parsed tree, node for node, before it is resolved.
//
// Each node is copied whole and then each child is replaced by its own copy,
// so a field the parser adds is kept, and no node is shared. err is the first
// node met that no case names; the copy goes on, and Compile refuses it.
type _Copier struct {
	err error
}

// _Copied is a copy of file that shares no node with it, so resolving one tree
// writes nothing into the other.
//
// file must not yet be resolved: a node is copied whole, and the copy of a
// resolved node would carry the resolver's binding.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Copied(file *syntax.File) (*syntax.File, error) {
	var (
		copier = new(_Copier)
		fresh  = *file
	)

	fresh.Stmts = copier._Stmts(file.Stmts)

	if copier.err != nil {
		return nil, copier.err
	}

	return &fresh, nil
}

// _Stmts copies a list of statements, keeping a nil list nil, since an if with
// no else is told apart by its nil False.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Stmts(stmts []syntax.Stmt) []syntax.Stmt {
	if stmts == nil {
		return nil
	}

	fresh := make(
		[]syntax.Stmt,
		len(stmts),
	)

	for idx, stmt := range stmts {
		fresh[idx] = c._Stmt(stmt)
	}

	return fresh
}

// _Stmt copies one statement.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Stmt(stmt syntax.Stmt) syntax.Stmt {
	switch stmt := stmt.(type) {
	case *syntax.AssignStmt:
		fresh := *stmt
		fresh.LHS = c._Expr(stmt.LHS)
		fresh.RHS = c._Expr(stmt.RHS)

		return &fresh

	case *syntax.BranchStmt:
		fresh := *stmt

		return &fresh

	case *syntax.DefStmt:
		fresh := *stmt
		fresh.Name = _Ident(stmt.Name)
		fresh.Params = c._Exprs(stmt.Params)
		fresh.Body = c._Stmts(stmt.Body)

		return &fresh

	case *syntax.ExprStmt:
		fresh := *stmt
		fresh.X = c._Expr(stmt.X)

		return &fresh

	case *syntax.ForStmt:
		fresh := *stmt
		fresh.Vars = c._Expr(stmt.Vars)
		fresh.X = c._Expr(stmt.X)
		fresh.Body = c._Stmts(stmt.Body)

		return &fresh

	case *syntax.IfStmt:
		fresh := *stmt
		fresh.Cond = c._Expr(stmt.Cond)
		fresh.True = c._Stmts(stmt.True)
		fresh.False = c._Stmts(stmt.False)

		return &fresh

	case *syntax.LoadStmt:
		return _Load(stmt)

	case *syntax.ReturnStmt:
		fresh := *stmt
		fresh.Result = c._Expr(stmt.Result)

		return &fresh

	case *syntax.WhileStmt:
		fresh := *stmt
		fresh.Cond = c._Expr(stmt.Cond)
		fresh.Body = c._Stmts(stmt.Body)

		return &fresh

	default:
		c._Refuse(stmt)

		return stmt
	}
}

// _Exprs copies a list of expressions, keeping a nil list nil.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Exprs(exprs []syntax.Expr) []syntax.Expr {
	if exprs == nil {
		return nil
	}

	fresh := make(
		[]syntax.Expr,
		len(exprs),
	)

	for idx, expr := range exprs {
		fresh[idx] = c._Expr(expr)
	}

	return fresh
}

// _Expr copies one expression, or keeps nil for an optional part left out.
//
// A name, a literal and an operator are here. The rest pass to _Postfix and
// then to _Composite, which keeps each switch one screen long.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Expr(expr syntax.Expr) syntax.Expr {
	if expr == nil {
		return nil
	}

	switch expr := expr.(type) {
	case *syntax.Ident:
		return _Ident(expr)

	case *syntax.Literal:
		fresh := *expr

		return &fresh

	case *syntax.ParenExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)

		return &fresh

	case *syntax.UnaryExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)

		return &fresh

	case *syntax.BinaryExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)
		fresh.Y = c._Expr(expr.Y)

		return &fresh

	case *syntax.CondExpr:
		fresh := *expr
		fresh.Cond = c._Expr(expr.Cond)
		fresh.True = c._Expr(expr.True)
		fresh.False = c._Expr(expr.False)

		return &fresh

	default:
		return c._Postfix(expr)
	}
}

// _Postfix copies an expression written after the operand it reads: an
// attribute, an index, a slice or a call.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Postfix(expr syntax.Expr) syntax.Expr {
	switch expr := expr.(type) {
	case *syntax.DotExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)
		fresh.Name = _Ident(expr.Name)

		return &fresh

	case *syntax.IndexExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)
		fresh.Y = c._Expr(expr.Y)

		return &fresh

	case *syntax.SliceExpr:
		fresh := *expr
		fresh.X = c._Expr(expr.X)
		fresh.Lo = c._Expr(expr.Lo)
		fresh.Hi = c._Expr(expr.Hi)
		fresh.Step = c._Expr(expr.Step)

		return &fresh

	case *syntax.CallExpr:
		fresh := *expr
		fresh.Fn = c._Expr(expr.Fn)
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
func (c *_Copier) _Composite(expr syntax.Expr) syntax.Expr {
	switch expr := expr.(type) {
	case *syntax.LambdaExpr:
		fresh := *expr
		fresh.Params = c._Exprs(expr.Params)
		fresh.Body = c._Expr(expr.Body)

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
		fresh.Key = c._Expr(expr.Key)
		fresh.Value = c._Expr(expr.Value)

		return &fresh

	case *syntax.Comprehension:
		fresh := *expr
		fresh.Body = c._Expr(expr.Body)
		fresh.Clauses = c._Clauses(expr.Clauses)

		return &fresh

	default:
		c._Refuse(expr)

		return expr
	}
}

// _Clauses copies the for and if clauses of a comprehension.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Clauses(clauses []syntax.Node) []syntax.Node {
	fresh := make(
		[]syntax.Node,
		len(clauses),
	)

	for idx, clause := range clauses {
		switch clause := clause.(type) {
		case *syntax.ForClause:
			fresh[idx] = c._For(clause)

		case *syntax.IfClause:
			fresh[idx] = c._If(clause)

		default:
			c._Refuse(clause)
			fresh[idx] = clause
		}
	}

	return fresh
}

// _For copies one for clause.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _For(clause *syntax.ForClause) *syntax.ForClause {
	fresh := *clause
	fresh.Vars = c._Expr(clause.Vars)
	fresh.X = c._Expr(clause.X)

	return &fresh
}

// _If copies one if clause.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _If(clause *syntax.IfClause) *syntax.IfClause {
	fresh := *clause
	fresh.Cond = c._Expr(clause.Cond)

	return &fresh
}

// _Refuse keeps the first node met that no case names.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func (c *_Copier) _Refuse(node syntax.Node) {
	if c.err != nil {
		return
	}

	c.err = fmt.Errorf(
		"%w: %T",
		ErrUncopied,
		node,
	)
}

// _Load copies a load statement: the module it names, the names it loads and
// the names it binds.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Load(load *syntax.LoadStmt) *syntax.LoadStmt {
	var (
		fresh  = *load
		module = *load.Module
	)

	fresh.Module = &module
	fresh.From = _Idents(load.From)
	fresh.To = _Loaded(
		load,
		fresh.From,
	)

	return &fresh
}

// _Idents copies a list of names.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Idents(idents []*syntax.Ident) []*syntax.Ident {
	fresh := make(
		[]*syntax.Ident,
		len(idents),
	)

	for idx, ident := range idents {
		fresh[idx] = _Ident(ident)
	}

	return fresh
}

// _Loaded copies the names a load binds, given the copies of the names it
// loads.
//
// The parser writes load("m", "x") as one name that is both loaded and bound,
// so where the two were one node, the copy is one node too.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Loaded(
	load *syntax.LoadStmt,
	from []*syntax.Ident,
) []*syntax.Ident {
	fresh := make(
		[]*syntax.Ident,
		len(load.To),
	)

	for idx, to := range load.To {
		fresh[idx] = from[idx]
		if to != load.From[idx] {
			fresh[idx] = _Ident(to)
		}
	}

	return fresh
}

// _Ident copies one name, the node the resolver writes a binding into.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Ident(ident *syntax.Ident) *syntax.Ident {
	fresh := *ident

	return &fresh
}
