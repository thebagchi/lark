package artifact

import (
	"fmt"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Signature is what a def takes, as a call's arguments are bound to it.
//
// positional is the parameters a call may fill by position, in order, and
// named every parameter a keyword may fill. required is those a call must
// fill. rest is a *args, which takes any number more by position, and
// keywords a **kwargs, which takes any keyword more.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Signature struct {
	positional []string
	named      map[string]bool
	required   map[string]bool
	rest       bool
	keywords   bool
}

// _Args refuses the first call, in any unit, that passes a function the
// script defines or loads arguments it does not take.
//
// Here rather than in the dialect, which compiles one file at a time: a loaded
// function's parameters are in another unit, and the compiler is what holds
// every unit. Only a call naming such a function is counted. A call that
// unpacks *args or **kwargs is not, since what it passes is known only when
// it runs.
//
// Returns spelling.ErrArity, opening with the call's file, line and column, as a parse
// or resolve error does.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-11-02 00:31: the walk of one unit moved to _Refusal
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 23:04: renamed from _Arity (C121, Q354)
func _Args(
	units map[string]*_Unit,
	order []string,
) error {
	for _, path := range order {
		var (
			unit    = units[path]
			refused = _Refusal(
				unit,
				_Signatures(
					units,
					unit,
				),
			)
		)
		if refused != nil {
			return refused
		}
	}

	return nil
}

// _Refusal is the first call in unit that _Fits refuses against signed, or nil
// when it holds none. The walk stops at the first.
//
// Revisions:
//   - 2026-11-02 00:31: initial creation, from _Arity: the walk of one unit
func _Refusal(
	unit *_Unit,
	signed map[*syntax.Ident]*_Signature,
) error {
	var refused error

	syntax.Walk(
		unit.tree,
		func(node syntax.Node) bool {
			call, ok := node.(*syntax.CallExpr)
			if ok && refused == nil {
				refused = _Fits(
					call,
					signed,
				)
			}

			return refused == nil
		},
	)

	return refused
}

// _Fits refuses call when it names a function in signed and passes it
// arguments that function does not take.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 16:05: cuts the file name in the position to common.QUOTE_LIMIT bytes (C117,
//     B239, Q4)
func _Fits(
	call *syntax.CallExpr,
	signed map[*syntax.Ident]*_Signature,
) error {
	name, ok := call.Fn.(*syntax.Ident)
	if !ok {
		return nil
	}

	bind, ok := name.Binding.(*resolve.Binding)
	if !ok {
		return nil
	}

	signature, ok := signed[bind.First]
	if !ok || _Unpacks(call) {
		return nil
	}

	if signature._Takes(call) {
		return nil
	}

	return fmt.Errorf(
		"%s: call of %s: %w",
		_Where(name.NamePos),
		name.Name,
		spelling.ErrArity,
	)
}

// _Signatures is what each function unit may call by name takes, keyed by
// where that name was first bound: a def at its top level other than the
// entry, and a name it loads that its module defines.
//
// Keyed by the binding rather than the name, so a parameter or a local that
// shadows a function is not counted as the function.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Signatures(
	units map[string]*_Unit,
	unit *_Unit,
) map[*syntax.Ident]*_Signature {
	signed := make(map[*syntax.Ident]*_Signature)

	for _, stmt := range unit.tree.Stmts {
		switch actual := stmt.(type) {
		case *syntax.DefStmt:
			if actual.Name.Name != common.ENTRY {
				signed[actual.Name] = _Signed(actual)
			}

		case *syntax.LoadStmt:
			module := units[unit.saved.GetLoads()[actual.ModuleName()]]

			for idx, to := range actual.To {
				def := _Defined(
					module,
					actual.From[idx].Name,
				)
				if def != nil {
					signed[to] = _Signed(def)
				}
			}
		}
	}

	return signed
}

// _Defined is the def module binds name to at its top level, or nil when it
// binds name to anything else, or there is no such module.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func _Defined(
	module *_Unit,
	name string,
) *syntax.DefStmt {
	if module == nil {
		return nil
	}

	for _, stmt := range module.tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok && def.Name.Name == name {
			return def
		}
	}

	return nil
}

// _Signed is what def takes.
//
// A parameter is required unless it has a default. One after a * or a *args
// can only be named, and a **kwargs takes no name of its own.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-11-02 00:31: the starred choice made once, as the place function
func _Signed(def *syntax.DefStmt) *_Signature {
	var (
		signature = &_Signature{
			named:    make(map[string]bool),
			required: make(map[string]bool),
		}
		place = signature._Add
	)

	for _, param := range def.Params {
		switch actual := param.(type) {
		case *syntax.Ident:
			place(actual.Name)

			signature.required[actual.Name] = true

		case *syntax.BinaryExpr:
			_Default(
				actual,
				place,
			)

		case *syntax.UnaryExpr:
			if signature._Star(actual) {
				place = signature._AddNamed
			}
		}
	}

	return signature
}

// _Default records the parameter that param gives a default to, with place.
// A default on anything but a name is none to record.
//
// Revisions:
//   - 2026-11-02 00:33: initial creation, from _Signed: the default case
func _Default(
	param *syntax.BinaryExpr,
	place func(string),
) {
	defaulted, ok := param.X.(*syntax.Ident)
	if ok {
		place(defaulted.Name)
	}
}

// _Star records a * or a **. It reports whether later names are keyword-only.
//
// A bare * only ends what a call may fill by position. A *args also takes
// any number more. ** does not, and the names after a * are keyword-only.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func (s *_Signature) _Star(actual *syntax.UnaryExpr) bool {
	if actual.Op == syntax.STARSTAR {
		s.keywords = true

		return false
	}

	s.rest = actual.X != nil

	return true
}

// _Add records a parameter a keyword may fill, and a call may fill by
// position unless it comes after a * or a *args.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-10-24 18:29: a name after * is recorded by _AddNamed
func (s *_Signature) _Add(
	name string,
) {
	s.named[name] = true
	s.positional = append(
		s.positional,
		name,
	)
}

// _AddNamed records a parameter only a keyword may fill.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func (s *_Signature) _AddNamed(
	name string,
) {
	s.named[name] = true
}

// _Takes reports whether call's arguments bind to this signature as Starlark
// binds them: by position first, then by name.
//
// The syntax tree holds a keyword as a BinaryExpr whose operator is =, so an
// operation such as a + 1 is a BinaryExpr too, and is passed by position.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
//   - 2026-10-08 17:28: counts an operation such as a + 1 by position, where it
//     read every BinaryExpr as a keyword
func (s *_Signature) _Takes(call *syntax.CallExpr) bool {
	var (
		filled = make(map[string]bool)
		passed = 0
	)

	for _, arg := range call.Args {
		keyword, ok := arg.(*syntax.BinaryExpr)
		if !ok || keyword.Op != syntax.EQ {
			passed++

			continue
		}

		name, ok := keyword.X.(*syntax.Ident)
		if !ok || !s._Fill(
			name.Name,
			filled,
		) {
			return false
		}
	}

	if passed > len(s.positional) && !s.rest {
		return false
	}

	for _, name := range s.positional[:min(
		passed,
		len(s.positional),
	)] {
		if filled[name] {
			return false
		}

		filled[name] = true
	}

	for name := range s.required {
		if !filled[name] {
			return false
		}
	}

	return true
}

// _Fill fills the parameter a keyword names, and reports whether the
// signature takes it: a parameter of that name not filled already, or any
// name at all when it takes **kwargs.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func (s *_Signature) _Fill(
	name string,
	filled map[string]bool,
) bool {
	if filled[name] {
		return false
	}

	if !s.named[name] {
		return s.keywords
	}

	filled[name] = true

	return true
}

// _Unpacks reports whether call passes *args or **kwargs, whose length is
// known only when it runs.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func _Unpacks(call *syntax.CallExpr) bool {
	for _, arg := range call.Args {
		unary, ok := arg.(*syntax.UnaryExpr)
		if ok && (unary.Op == syntax.STAR || unary.Op == syntax.STARSTAR) {
			return true
		}
	}

	return false
}
