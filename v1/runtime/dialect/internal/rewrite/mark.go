package rewrite

import (
	"strconv"
	"sync"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect/internal/spec"
)

// Mark is what the compiled tree changes about one call: whether it goes
// through CALL, which reports it, and the hidden keywords it gains. guard holds
// every change to the mark and to the call it rewrites, so goroutines that
// share a mark do not race.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 10:35: the guard field makes changes under it safe (C108, B210, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Mark to Mark; renamed the
//     fields reported to Reported, for the tests in tests/ (C120b, Q315, Q314)
type Mark struct {
	guard    sync.Mutex
	Reported bool
	keywords []syntax.Expr
}

// Marker reads the written tree for the calls the compiled tree changes.
//
// scripts is where each function the script may call by name was first bound,
// and marks is what was found, by where each call's parenthesis opens.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Marker to Marker; renamed
//     the fields marks to Marks, for the tests in tests/ (C120b, Q315, Q314)
type Marker struct {
	scripts map[*syntax.Ident]bool
	Marks   map[At]*Mark
}

// Marks is every call the compiled tree changes, read off the written tree,
// which is resolved, by where each call's parenthesis opens.
//
// A nil file holds no call.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 10:35: a nil file marks nothing (C108, graph-F42)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Marks to Marks, for the
//     tests in tests/ (C120b, Q315, Q314)
func Marks(written *syntax.File) map[At]*Mark {
	if written == nil {
		return make(map[At]*Mark)
	}

	marker := &Marker{
		scripts: Scripts(written),
		Marks:   make(map[At]*Mark),
	}

	marker._List(
		written.Stmts,
		"",
	)
	marker.Lambdas(written)

	return marker.Marks
}

// Assign marks a statement binding one name to a call: the call, as any
// statement call is marked, and the name when the call is a spawn, since the
// thread it starts is reported under that name.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: a nil statement marks nothing (C108, B206)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Assign to Assign, for the
//     tests in tests/ (C120b, Q315, Q314)
func (m *Marker) Assign(
	stmt *syntax.AssignStmt,
	arm string,
) {
	if stmt == nil {
		return
	}

	name, ok := stmt.LHS.(*syntax.Ident)
	if !ok || stmt.Op != syntax.EQ {
		return
	}

	call, ok := stmt.RHS.(*syntax.CallExpr)
	if !ok {
		return
	}

	m._Call(
		call,
		arm,
	)

	if _Builtin(call.Fn) == common.SPAWN {
		m.For(call)._Pass(
			common.BINDING,
			name.Name,
			call.Lparen,
		)
	}
}

// Lambdas marks each builtin that runs a function, wherever its call is
// written, with the function the lambda it is handed calls.
//
// spawn(lambda: task_a("west")) starts a thread whose function is task_a, and
// retry(2, lambda: attempt("x")) reports attempt: a lambda is only how they
// pass arguments, and its own name says nothing.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: a nil file marks nothing (C108, B207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Lambdas to Lambdas, for the
//     tests in tests/ (C120b, Q315, Q314)
func (m *Marker) Lambdas(written *syntax.File) {
	if written == nil {
		return
	}

	syntax.Walk(
		written,
		func(node syntax.Node) bool {
			call, ok := node.(*syntax.CallExpr)
			if !ok {
				return true
			}

			callee := m._Called(Handed(call))
			if callee != "" {
				m.For(call)._Pass(
					common.CALLEE,
					callee,
					call.Lparen,
				)
			}

			return true
		},
	)
}

// For is the mark of call, made when it has none yet. A nil call has a mark
// the marker does not keep, so what is passed to it is dropped.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 10:35: a nil call has a loose mark, and a missing map is made (C108, B208, B209)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _For to For, for the tests in
//     tests/ (C120b, Q315, Q314)
func (m *Marker) For(call *syntax.CallExpr) *Mark {
	if call == nil {
		return new(Mark)
	}

	if m.Marks == nil {
		m.Marks = make(map[At]*Mark)
	}

	var at = _Where(call.Lparen)

	mark, ok := m.Marks[at]
	if !ok {
		mark = new(Mark)
		m.Marks[at] = mark
	}

	return mark
}

// _List marks the calls in one statement list and in every list inside it.
//
// branch is the builtin the list is an arm of, if or match, or empty for any
// other list. An arm that is one statement once its closer is set aside makes
// that statement's call the branch's, which is the shape derive reads as an If
// or a Match. An arm of two statements is neither, and its calls are plain.
//
// A match's subject is not marked: classify() in _match = classify() is what
// the match compares, not a line of its own.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _List(
	stmts []syntax.Stmt,
	branch string,
) {
	arm := ""
	if len(Closed(stmts)) == 1 {
		arm = branch
	}

	for idx := 0; idx < len(stmts); idx++ {
		_, chain, matched := Matched(
			stmts,
			idx,
		)
		if matched {
			m._Chain(chain)

			idx++

			continue
		}

		m._Statement(
			stmts[idx],
			arm,
		)
	}
}

// _Statement marks the calls in one statement: its own call when it is one,
// and those of every list it holds.
//
// A call is a statement alone or bound to one name, which is what derive reads
// as a Call. A loop's body and a nested def's are lists of their own and no
// branch's, even inside one: a loop in a branch reports a line per iteration,
// as any loop does.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _Statement(
	stmt syntax.Stmt,
	arm string,
) {
	switch actual := stmt.(type) {
	case *syntax.ExprStmt:
		m._Call(
			actual.X,
			arm,
		)

	case *syntax.AssignStmt:
		m.Assign(
			actual,
			arm,
		)

	case *syntax.IfStmt:
		m._List(
			actual.True,
			common.IF,
		)
		m._List(
			actual.False,
			common.IF,
		)

	case *syntax.ForStmt:
		m._List(
			actual.Body,
			"",
		)

	case *syntax.WhileStmt:
		m._List(
			actual.Body,
			"",
		)

	case *syntax.DefStmt:
		m._List(
			actual.Body,
			"",
		)
	}
}

// _Chain marks the arms of a match's chain: each case, and the default.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _Chain(chain *syntax.IfStmt) {
	m._List(
		chain.True,
		common.MATCH,
	)

	next := NextCase(chain)
	if next != nil {
		m._Chain(next)

		return
	}

	m._List(
		chain.False,
		common.MATCH,
	)
}

// _Call marks a call of a function the script defines or loads as one that
// goes through CALL, with the branch it is the arm of when it is one.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 12:59: marks the call as reported under the guard (C108, B210, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _Call(
	expr syntax.Expr,
	arm string,
) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return
	}

	if _, script := m._Script(call.Fn); !script {
		return
	}

	mark := m.For(call)

	mark.Report()

	if arm != "" {
		mark._Pass(
			common.BUILTIN,
			arm,
			call.Lparen,
		)
	}
}

// _Called is the function the script defines or loads that a lambda calls as
// its whole body, or empty: lambda: task_a("west") calls task_a, and a lambda
// whose body is anything else names nothing.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _Called(expr syntax.Expr) string {
	lambda, ok := expr.(*syntax.LambdaExpr)
	if !ok {
		return ""
	}

	call, ok := lambda.Body.(*syntax.CallExpr)
	if !ok {
		return ""
	}

	name, script := m._Script(call.Fn)
	if !script {
		return ""
	}

	return name
}

// _Script is the name expr calls, and whether it names a function the script
// defines at its top level or loads, read off where the name was first bound.
//
// The resolver has already settled what a name means: a parameter or a local
// of a def's name is bound inside its function, so it is not the def it
// shadows.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (m *Marker) _Script(expr syntax.Expr) (string, bool) {
	name, ok := expr.(*syntax.Ident)
	if !ok {
		return "", false
	}

	bind, ok := name.Binding.(*resolve.Binding)
	if !ok || !m.scripts[bind.First] {
		return "", false
	}

	return name.Name, true
}

// Report marks the call as one that goes through CALL, under the guard.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B210, Q207)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Report to Report, for the
//     tests in tests/ (C120b, Q315, Q314)
func (mark *Mark) Report() {
	mark.guard.Lock()
	defer mark.guard.Unlock()

	mark.Reported = true
}

// Change rewrites call as marked.
//
// A reported call becomes a call of CALL with the function first, so that CALL
// reports it. The hidden keywords go after every keyword the call passes
// already and before any *args or **kwargs, the last place the resolver allows
// a keyword.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: rewrites the call under the guard (C108, B210)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Change to Change, for the
//     tests in tests/ (C120b, Q315, Q314)
func (mark *Mark) Change(call *syntax.CallExpr) {
	mark.guard.Lock()
	defer mark.guard.Unlock()

	args := call.Args

	if mark.Reported {
		args = append(
			[]syntax.Expr{call.Fn},
			args...,
		)
		call.Fn = &syntax.Ident{
			NamePos: syntax.Start(call.Fn),
			Name:    common.CALL,
		}
	}

	call.Args = _Keyworded(
		args,
		mark.keywords,
	)
}

// _Pass adds the hidden keyword name = value to what the call passes.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 10:35: adds under the guard (C108, B210)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func (mark *Mark) _Pass(
	name string,
	value string,
	at syntax.Position,
) {
	mark.guard.Lock()
	defer mark.guard.Unlock()

	mark.keywords = append(
		mark.keywords,
		_Keyword(
			name,
			value,
			at,
		),
	)
}

// _Apply changes every marked call in the compiled tree.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Apply(
	compiled *syntax.File,
	marks map[At]*Mark,
) {
	if len(marks) == 0 {
		return
	}

	syntax.Walk(
		compiled,
		func(node syntax.Node) bool {
			call, ok := node.(*syntax.CallExpr)
			if !ok {
				return true
			}

			mark, ok := marks[_Where(call.Lparen)]
			if ok {
				mark.Change(call)
			}

			return true
		},
	)
}

// Scripts is where each function a script may call by name was first bound:
// every def at its top level but the entry, and every name a load binds.
//
// The entry is left out because a flow does not list it: a call of main is no
// line of anything.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: a nil file, a nil node or a nil name binds nothing (C108, B211, B212)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Scripts to Scripts, for the
//     tests in tests/ (C120b, Q315, Q314)
func Scripts(written *syntax.File) map[*syntax.Ident]bool {
	found := make(map[*syntax.Ident]bool)

	if written == nil {
		return found
	}

	for _, stmt := range written.Stmts {
		switch actual := stmt.(type) {
		case *syntax.DefStmt:
			if _Named(actual) && actual.Name.Name != common.ENTRY {
				found[actual.Name] = true
			}

		case *syntax.LoadStmt:
			if actual == nil {
				continue
			}

			for _, to := range actual.To {
				if to != nil {
					found[to] = true
				}
			}
		}
	}

	return found
}

// _Named reports whether def is there and has a name.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B212)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Named(def *syntax.DefStmt) bool {
	return def != nil && def.Name != nil
}

// Handed is the function a call hands the builtin that runs it, or nil when
// the call names no such builtin or does not pass the function by position.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 10:35: a nil call hands nothing (C108, B213)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Handed to Handed, for the
//     tests in tests/ (C120b, Q315, Q314)
func Handed(call *syntax.CallExpr) syntax.Expr {
	if call == nil {
		return nil
	}

	position, ok := spec.HANDED[_Builtin(call.Fn)]
	if !ok || position >= len(call.Args) {
		return nil
	}

	return call.Args[position]
}

// _Builtin is the name of the environment's builtin expr names, or empty when
// it names anything else, a function of the script's own included.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Builtin(expr syntax.Expr) string {
	name, ok := expr.(*syntax.Ident)
	if !ok || !_Predeclared(name) {
		return ""
	}

	return name.Name
}

// _Keyword is the keyword argument name = value, placed where the call it is
// passed to opens, so an error about it points at that call.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Keyword(
	name string,
	value string,
	at syntax.Position,
) syntax.Expr {
	return &syntax.BinaryExpr{
		X: &syntax.Ident{
			NamePos: at,
			Name:    name,
		},
		OpPos: at,
		Op:    syntax.EQ,
		Y: &syntax.Literal{
			Token:    syntax.STRING,
			TokenPos: at,
			Raw:      strconv.Quote(value),
			Value:    value,
		},
	}
}

// _Keyworded is args with keywords placed after the keywords already there and
// before any *args or **kwargs.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Keyworded(
	args []syntax.Expr,
	keywords []syntax.Expr,
) []syntax.Expr {
	if len(keywords) == 0 {
		return args
	}

	cut := len(args)

	for idx, arg := range args {
		if _Unpacked(arg) {
			cut = idx

			break
		}
	}

	placed := make(
		[]syntax.Expr,
		0,
		len(args)+len(keywords),
	)
	placed = append(
		placed,
		args[:cut]...,
	)
	placed = append(
		placed,
		keywords...,
	)

	return append(
		placed,
		args[cut:]...,
	)
}

// _Unpacked reports whether a call's argument is *args or **kwargs.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Unpacked(arg syntax.Expr) bool {
	unary, ok := arg.(*syntax.UnaryExpr)

	return ok && (unary.Op == syntax.STAR || unary.Op == syntax.STARSTAR)
}
