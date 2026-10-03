package dialect

import (
	"strconv"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// HANDED is where each builtin that runs a function it is handed takes that
// function: spawn first, and each wrapper after the count or the budget it
// takes first.
var HANDED = map[string]int{
	spelling.SPAWN:   0,
	spelling.REPEAT:  1,
	spelling.RETRY:   1,
	spelling.TIMEOUT: 1,
}

// _Mark is what the compiled tree changes about one call: whether it goes
// through CALL, which reports it, and the hidden keywords it gains.
type _Mark struct {
	reported bool
	keywords []syntax.Expr
}

// _Marker reads the written tree for the calls the compiled tree changes.
//
// scripts is where each function the script may call by name was first bound,
// and marks is what was found, by where each call's parenthesis opens.
type _Marker struct {
	scripts map[*syntax.Ident]bool
	marks   map[_At]*_Mark
}

// _Marks is every call the compiled tree changes, read off the written tree,
// which is resolved, by where each call's parenthesis opens.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func _Marks(written *syntax.File) map[_At]*_Mark {
	marker := &_Marker{
		scripts: _Scripts(written),
		marks:   make(map[_At]*_Mark),
	}

	marker._List(written.Stmts, "")
	marker._Lambdas(written)

	return marker.marks
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
func (m *_Marker) _List(stmts []syntax.Stmt, branch string) {
	arm := ""
	if len(Closed(stmts)) == 1 {
		arm = branch
	}

	for idx := 0; idx < len(stmts); idx++ {
		_, chain, matched := Matched(stmts, idx)
		if matched {
			m._Chain(chain)
			idx++

			continue
		}

		m._Statement(stmts[idx], arm)
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
func (m *_Marker) _Statement(stmt syntax.Stmt, arm string) {
	switch actual := stmt.(type) {
	case *syntax.ExprStmt:
		m._Call(actual.X, arm)

	case *syntax.AssignStmt:
		m._Assign(actual, arm)

	case *syntax.IfStmt:
		m._List(actual.True, spelling.IF)
		m._List(actual.False, spelling.IF)

	case *syntax.ForStmt:
		m._List(actual.Body, "")

	case *syntax.WhileStmt:
		m._List(actual.Body, "")

	case *syntax.DefStmt:
		m._List(actual.Body, "")
	}
}

// _Chain marks the arms of a match's chain: each case, and the default.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _Chain(chain *syntax.IfStmt) {
	m._List(chain.True, spelling.MATCH)

	next := NextCase(chain)
	if next != nil {
		m._Chain(next)

		return
	}

	m._List(chain.False, spelling.MATCH)
}

// _Assign marks a statement binding one name to a call: the call, as any
// statement call is marked, and the name when the call is a spawn, since the
// thread it starts is reported under that name.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _Assign(stmt *syntax.AssignStmt, arm string) {
	name, ok := stmt.LHS.(*syntax.Ident)
	if !ok || stmt.Op != syntax.EQ {
		return
	}

	call, ok := stmt.RHS.(*syntax.CallExpr)
	if !ok {
		return
	}

	m._Call(call, arm)

	if _Builtin(call.Fn) == spelling.SPAWN {
		m._For(call)._Pass(spelling.BINDING, name.Name, call.Lparen)
	}
}

// _Call marks a call of a function the script defines or loads as one that
// goes through CALL, with the branch it is the arm of when it is one.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _Call(expr syntax.Expr, arm string) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return
	}

	if _, script := m._Script(call.Fn); !script {
		return
	}

	mark := m._For(call)
	mark.reported = true

	if arm != "" {
		mark._Pass(spelling.BUILTIN, arm, call.Lparen)
	}
}

// _Lambdas marks each builtin that runs a function, wherever its call is
// written, with the function the lambda it is handed calls.
//
// spawn(lambda: task_a("west")) starts a thread whose function is task_a, and
// retry(2, lambda: attempt("x")) reports attempt: a lambda is only how they
// pass arguments, and its own name says nothing.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _Lambdas(written *syntax.File) {
	syntax.Walk(written, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}

		callee := m._Called(_Handed(call))
		if callee != "" {
			m._For(call)._Pass(spelling.CALLEE, callee, call.Lparen)
		}

		return true
	})
}

// _Called is the function the script defines or loads that a lambda calls as
// its whole body, or empty: lambda: task_a("west") calls task_a, and a lambda
// whose body is anything else names nothing.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _Called(expr syntax.Expr) string {
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
func (m *_Marker) _Script(expr syntax.Expr) (string, bool) {
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

// _For is the mark of call, made when it has none yet.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (m *_Marker) _For(call *syntax.CallExpr) *_Mark {
	at := _Where(call.Lparen)

	mark, ok := m.marks[at]
	if !ok {
		mark = new(_Mark)
		m.marks[at] = mark
	}

	return mark
}

// _Pass adds the hidden keyword name = value to what the call passes.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (mark *_Mark) _Pass(name string, value string, at syntax.Position) {
	mark.keywords = append(mark.keywords, _Keyword(name, value, at))
}

// _Change rewrites call as marked.
//
// A reported call becomes a call of CALL with the function first, so that CALL
// reports it. The hidden keywords go after every keyword the call passes
// already and before any *args or **kwargs, the last place the resolver allows
// a keyword.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func (mark *_Mark) _Change(call *syntax.CallExpr) {
	args := call.Args

	if mark.reported {
		args = append([]syntax.Expr{call.Fn}, args...)
		call.Fn = &syntax.Ident{NamePos: syntax.Start(call.Fn), Name: spelling.CALL}
	}

	call.Args = _Keyworded(args, mark.keywords)
}

// _Apply changes every marked call in the compiled tree.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func _Apply(compiled *syntax.File, marks map[_At]*_Mark) {
	if len(marks) == 0 {
		return
	}

	syntax.Walk(compiled, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}

		mark, ok := marks[_Where(call.Lparen)]
		if ok {
			mark._Change(call)
		}

		return true
	})
}

// _Scripts is where each function a script may call by name was first bound:
// every def at its top level but the entry, and every name a load binds.
//
// The entry is left out because a flow does not list it: a call of main is no
// line of anything.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func _Scripts(written *syntax.File) map[*syntax.Ident]bool {
	found := make(map[*syntax.Ident]bool)

	for _, stmt := range written.Stmts {
		switch actual := stmt.(type) {
		case *syntax.DefStmt:
			if actual.Name.Name != spelling.ENTRY {
				found[actual.Name] = true
			}

		case *syntax.LoadStmt:
			for _, to := range actual.To {
				found[to] = true
			}
		}
	}

	return found
}

// _Handed is the function a call hands the builtin that runs it, or nil when
// the call names no such builtin or does not pass the function by position.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func _Handed(call *syntax.CallExpr) syntax.Expr {
	position, ok := HANDED[_Builtin(call.Fn)]
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
func _Keyword(name string, value string, at syntax.Position) syntax.Expr {
	return &syntax.BinaryExpr{
		X:     &syntax.Ident{NamePos: at, Name: name},
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
func _Keyworded(args []syntax.Expr, keywords []syntax.Expr) []syntax.Expr {
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

	placed := make([]syntax.Expr, 0, len(args)+len(keywords))
	placed = append(placed, args[:cut]...)
	placed = append(placed, keywords...)

	return append(placed, args[cut:]...)
}

// _Unpacked reports whether a call's argument is *args or **kwargs.
//
// Revisions:
//   - 2026-10-02 01:16: initial creation
func _Unpacked(arg syntax.Expr) bool {
	unary, ok := arg.(*syntax.UnaryExpr)

	return ok && (unary.Op == syntax.STAR || unary.Op == syntax.STARSTAR)
}
