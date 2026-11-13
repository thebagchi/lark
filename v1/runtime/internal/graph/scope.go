package graph

import (
	"sync"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/dialect"
)

// Scope is what one statement list can see while it is read: the enclosing
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
//
// guard holds every write to and read of the four maps, so a goroutine that
// writes one while another reads it does not race. A writer that reaches a map
// from outside this file takes guard first. A list inside a statement shares
// params and locals with the list around it, so home names the scope whose
// guard holds them; a writer takes the guard of that scope.
//
// Revisions:
//   - 2026-11-05 23:24: the lines field is a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-08 10:35: the guard and home fields make writes to the maps safe
//     (C108, B188, Q207)
//   - 2026-11-10 00:25: renamed from _Scope and the fields guard, locals, outer, params, reading,
//     spawns, top, values, as a test in tests/ needs them (C120a, Q314)
type Scope struct {
	Guard   sync.RWMutex
	Reading *Reading
	Outer   *Scope
	home    *Scope
	Params  map[string]bool
	Locals  map[string]bool
	Values  map[string]bool
	Spawns  map[string]bool
	Top     bool
	lines   *Lines
}

// _Opened is the scope of a function's own statement list.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-10 01:00: now uses Reading, Scope, Reading, Params, and 3 more, were _Reading,
//     _Scope, reading, params, and 3 more (C120a, Q314)
func (r *Reading) _Opened(
	def *syntax.DefStmt,
	params []string,
) *Scope {
	scope := &Scope{
		Reading: r,
		Params:  make(map[string]bool),
		Locals:  make(map[string]bool),
		Values:  make(map[string]bool),
		Spawns:  make(map[string]bool),
	}

	for _, name := range params {
		scope.Params[name] = true
	}

	dialect.Bound(
		def.Body,
		scope.Locals,
	)

	return scope
}

// _Top is the scope a module-level constant is computed in.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-10 01:00: now uses Reading, Scope, Reading, Top, were _Reading, _Scope, reading, top
//     (C120a, Q314)
func (r *Reading) _Top() *Scope {
	return &Scope{
		Reading: r,
		Top:     true,
	}
}

// _Initing is the scope an entry's top-level statement is read in: the top
// scope, with the lines of the file, which an argument that is an expression
// is cut from.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-05 23:24: takes a *Lines in place of a *_Lines (C60, Q188)
//   - 2026-11-10 01:00: now uses Reading, Scope, Reading, Top, were _Reading, _Scope, reading, top
//     (C120a, Q314)
func (r *Reading) _Initing(lines *Lines) *Scope {
	return &Scope{
		Reading: r,
		Top:     true,
		lines:   lines,
	}
}

// Inner is the scope of a list inside a statement on this one.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-06 22:02: a nil scope has a nil inner scope (C100c, B148)
//   - 2026-11-08 12:59: names the scope that owns the shared maps (C108, B188, Q207)
//   - 2026-11-10 00:25: renamed from _Inner, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Inner() *Scope {
	if s == nil {
		return nil
	}

	return &Scope{
		Reading: s.Reading,
		Outer:   s,
		home:    s._Home(),
		Params:  s.Params,
		Locals:  s.Locals,
		Values:  make(map[string]bool),
		Spawns:  make(map[string]bool),
		Top:     s.Top,
	}
}

// Sees reports whether an operand, a condition or a match may name name here.
//
// In order: a result or a spawn binding this list or one around it holds, a
// parameter, then a function, constant or argument the flow declares - unless
// the function assigns the name somewhere this list cannot see.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-06 22:02: a nil scope sees nothing (C100c, B148)
//   - 2026-11-08 10:35: reads under the guard, and ends the walk of a cyclic chain
//     (C108, B187, B188)
//   - 2026-11-10 00:25: renamed from _Sees, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Sees(name string) bool {
	if s == nil {
		return false
	}

	if s.Top {
		return s.Reading._Prior(name)
	}

	if s._Held(
		name,
		(*Scope).Binds,
	) {
		return true
	}

	param, local := s._Declared(name)
	if param {
		return true
	}

	if local {
		return false
	}

	return s.Reading._Global(name)
}

// Binds reports whether this list alone holds name as a result or as a spawn
// binding.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B188)
//   - 2026-11-10 00:25: renamed from _Binds, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Binds(name string) bool {
	s.Guard.RLock()
	defer s.Guard.RUnlock()

	return s.Values[name] || s.Spawns[name]
}

// Forks reports whether this list alone holds name as a spawn binding.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B188)
//   - 2026-11-10 00:25: renamed from _Forks, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Forks(name string) bool {
	s.Guard.RLock()
	defer s.Guard.RUnlock()

	return s.Spawns[name]
}

// Spawned reports whether name is a spawn binding this list or one around it
// holds, which is what a join or a cancel must name.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-08 10:35: walks through _Held, which ends on a cyclic chain (C108, B187)
//   - 2026-11-10 00:25: renamed from _Spawned, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Spawned(name string) bool {
	return s._Held(
		name,
		(*Scope).Forks,
	)
}

// Callable reports whether a call of name here calls a function the flow
// lists, rather than a parameter or a local of that name.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-06 22:02: a nil scope calls nothing (C100c, B150)
//   - 2026-11-10 00:25: renamed from _Callable, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Callable(name string) bool {
	return s != nil && !s._Shadows(name) && s.Reading.Listed(name)
}

// Usable reports whether name here means the builtin of that name: nothing
// the script declares or the function binds is called that.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-10 00:25: renamed from _Usable, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Usable(name string) bool {
	return !s._Shadows(name) && s.Reading.Builtin(name)
}

// Bind records a result this list binds.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-08 10:35: writes under the guard (C108, B188)
//   - 2026-11-10 00:25: renamed from _Bind, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Bind(name string) {
	s.Guard.Lock()
	defer s.Guard.Unlock()

	s.Values[name] = true
}

// Fork records a spawn binding this list binds, and reports false when this
// list has bound that name to a spawn already: the second would hide the
// first, and a join could not say which.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-08 10:35: tests and sets the binding under the guard (C108, B188)
//   - 2026-11-10 00:25: renamed from _Fork, as a test in tests/ needs it (C120a, Q314)
func (s *Scope) Fork(name string) bool {
	s.Guard.Lock()
	defer s.Guard.Unlock()

	if s.Spawns[name] {
		return false
	}

	s.Spawns[name] = true

	return true
}

// _Held reports whether holds says so of name for this list or one around it.
//
// Two pointers walk the chain, one at half the pace of the other, so a chain
// that loops back on itself ends the walk once every scope on it was read. The
// parser never builds such a chain; a hand-built one would hang the walk.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation, the loop _Sees and _Spawned shared (C108, B187)
//   - 2026-11-10 01:00: now uses Scope, Outer, were _Scope, outer (C120a, Q314)
func (s *Scope) _Held(
	name string,
	holds func(
		*Scope,
		string,
	) bool,
) bool {
	var (
		slow = s
		odd  bool
	)

	for scope := s; scope != nil; odd = !odd {
		if holds(
			scope,
			name,
		) {
			return true
		}

		scope = scope.Outer

		if odd {
			slow = slow.Outer
		}

		if scope == slow {
			break
		}
	}

	return false
}

// _Declared reports whether the function declares name as a parameter, and
// whether it assigns name as a local, in that order.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B188)
//   - 2026-11-10 01:00: now uses Scope, Guard, Params, Locals, were _Scope, guard, params, locals
//     (C120a, Q314)
func (s *Scope) _Declared(name string) (bool, bool) {
	home := s._Home()

	home.Guard.RLock()
	defer home.Guard.RUnlock()

	return s.Params[name], s.Locals[name]
}

// _Home is the scope whose guard holds the params and locals this scope reads:
// the scope of the function's own list, or this one when it is that.
//
// Revisions:
//   - 2026-11-08 12:59: initial creation (C108, B188, Q207)
//   - 2026-11-10 01:00: now uses Scope, were _Scope (C120a, Q314)
func (s *Scope) _Home() *Scope {
	if s.home != nil {
		return s.home
	}

	return s
}

// Shadows reports whether the function binds name itself, as a parameter or a
// local.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-08 10:35: reads under the guard (C108, B188)
//   - 2026-11-10 01:00: now uses Shadows, Scope, Top, were _Shadows, _Scope, top (C120a, Q314)
func (s *Scope) _Shadows(name string) bool {
	param, local := s._Declared(name)

	return !s.Top && (param || local)
}

// _Global reports whether name is a function, a constant or an argument the
// flow declares.
//
// Revisions:
//   - 2026-10-02 00:04: initial creation
//   - 2026-11-08 12:46: a nil reading declares nothing (C108, B187)
//   - 2026-11-10 01:00: now uses Reading, Listed, were _Reading, _Listed (C120a, Q314)
func (r *Reading) _Global(name string) bool {
	if r == nil {
		return false
	}

	var (
		constant bool
		arg      bool
	)

	_, constant = r.constants[name]
	_, arg = r.args[name]

	return constant || arg || r.Listed(name)
}
