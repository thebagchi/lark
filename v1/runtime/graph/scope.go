package graph

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// _Scope is what one statement list can see while it is read: the enclosing
// function's parameters, the results and spawn bindings earlier on this list
// and on every list around it, and what the flow declares at its top level.
//
// A list inside a statement - a branch, a case, a loop's body - sees the list
// around it as it stood at that statement, and what it binds does not leak
// out. So a name bound inside a branch and used after it resolves to nothing,
// and the function keeps its text: Check would refuse the flow otherwise.
//
// locals is every name the function assigns anywhere. Starlark makes such a
// name local to the whole function, so where this list cannot see the binding
// it does not fall back to a constant of the same name either.
//
// top is the module's own scope, where a constant is computed: it sees the
// functions and the constants declared before it, and nothing else.
type _Scope struct {
	reading *_Reading
	outer   *_Scope
	params  map[string]bool
	locals  map[string]bool
	values  map[string]bool
	spawns  map[string]bool
	top     bool
}

// _Opened is the scope of a function's own statement list.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Opened(def *syntax.DefStmt, params []string) *_Scope {
	scope := &_Scope{
		reading: r,
		params:  make(map[string]bool),
		locals:  make(map[string]bool),
		values:  make(map[string]bool),
		spawns:  make(map[string]bool),
	}

	for _, name := range params {
		scope.params[name] = true
	}

	dialect.Bound(def.Body, scope.locals)

	return scope
}

// _Top is the scope a module-level constant is computed in.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Top() *_Scope {
	return &_Scope{reading: r, top: true}
}

// _Inner is the scope of a list inside a statement on this one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Inner() *_Scope {
	return &_Scope{
		reading: s.reading,
		outer:   s,
		params:  s.params,
		locals:  s.locals,
		values:  make(map[string]bool),
		spawns:  make(map[string]bool),
		top:     s.top,
	}
}

// _Sees reports whether an operand, a condition or a match may name name here.
//
// In order: a result or a spawn binding this list or one around it holds, a
// parameter, then a function, constant or argument the flow declares - unless
// the function assigns the name somewhere this list cannot see.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Sees(name string) bool {
	if s.top {
		return s.reading._Prior(name)
	}

	for scope := s; scope != nil; scope = scope.outer {
		if scope.values[name] || scope.spawns[name] {
			return true
		}
	}

	if s.params[name] {
		return true
	}

	if s.locals[name] {
		return false
	}

	return s.reading._Global(name)
}

// _Spawned reports whether name is a spawn binding this list or one around it
// holds, which is what a join or a cancel must name.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Spawned(name string) bool {
	for scope := s; scope != nil; scope = scope.outer {
		if scope.spawns[name] {
			return true
		}
	}

	return false
}

// _Callable reports whether a call of name here calls a function the flow
// lists, rather than a parameter or a local of that name.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Callable(name string) bool {
	return !s._Shadows(name) && s.reading._Listed(name)
}

// _Usable reports whether name here means the builtin of that name: nothing
// the script declares or the function binds is called that.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Usable(name string) bool {
	return !s._Shadows(name) && s.reading._Builtin(name)
}

// _Shadows reports whether the function binds name itself, as a parameter or a
// local.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Shadows(name string) bool {
	return !s.top && (s.params[name] || s.locals[name])
}

// _Bind records a result this list binds.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Bind(name string) {
	s.values[name] = true
}

// _Fork records a spawn binding this list binds, and reports false when this
// list has bound that name to a spawn already: the second would hide the
// first, and a join could not say which.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (s *_Scope) _Fork(name string) bool {
	if s.spawns[name] {
		return false
	}

	s.spawns[name] = true

	return true
}

// _Global reports whether name is a function, a constant or an argument the
// flow declares.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
func (r *_Reading) _Global(name string) bool {
	_, constant := r.constants[name]
	_, arg := r.args[name]

	return constant || arg || r._Listed(name)
}
