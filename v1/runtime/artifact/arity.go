package artifact

import (
	"fmt"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// ERR_ARITY is returned for a call that passes a function the script defines
// or loads arguments it does not take: too many, too few, one it has no
// parameter for, or one twice. spelling's, so it is the value a flow's check
// refuses the same mistake with.
var ERR_ARITY = spelling.ERR_ARITY

// _Signature is what a def takes, as a call's arguments are bound to it.
//
// positional is the parameters a call may fill by position, in order, and
// named every parameter a keyword may fill. required is those a call must
// fill. rest is a *args, which takes any number more by position, and
// keywords a **kwargs, which takes any keyword more.
type _Signature struct {
	positional []string
	named      map[string]bool
	required   map[string]bool
	rest       bool
	keywords   bool
}

// _Arity refuses the first call, in any unit, that passes a function the
// script defines or loads arguments it does not take.
//
// Here rather than in the dialect, which compiles one file at a time: a loaded
// function's parameters are in another unit, and the compiler is what holds
// every unit. Only a call naming such a function is counted. A call that
// unpacks *args or **kwargs is not, since what it passes is known only when
// it runs.
//
// Returns ERR_ARITY, opening with the call's file, line and column, as a parse
// or resolve error does.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func _Arity(units map[string]*_Unit, order []string) error {
	for _, path := range order {
		unit := units[path]
		signed := _Signatures(units, unit)

		var refused error

		syntax.Walk(unit.tree, func(node syntax.Node) bool {
			call, ok := node.(*syntax.CallExpr)
			if ok && refused == nil {
				refused = _Fits(call, signed)
			}

			return refused == nil
		})

		if refused != nil {
			return refused
		}
	}

	return nil
}

// _Fits refuses call when it names a function in signed and passes it
// arguments that function does not take.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func _Fits(call *syntax.CallExpr, signed map[*syntax.Ident]*_Signature) error {
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

	return fmt.Errorf("%s: call of %s: %w", name.NamePos, name.Name, ERR_ARITY)
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
func _Signatures(units map[string]*_Unit, unit *_Unit) map[*syntax.Ident]*_Signature {
	signed := make(map[*syntax.Ident]*_Signature)

	for _, stmt := range unit.tree.Stmts {
		switch actual := stmt.(type) {
		case *syntax.DefStmt:
			if actual.Name.Name != ENTRY {
				signed[actual.Name] = _Signed(actual)
			}

		case *syntax.LoadStmt:
			module := units[unit.saved.GetLoads()[actual.ModuleName()]]

			for idx, to := range actual.To {
				def := _Defined(module, actual.From[idx].Name)
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
func _Defined(module *_Unit, name string) *syntax.DefStmt {
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
func _Signed(def *syntax.DefStmt) *_Signature {
	signature := &_Signature{
		named:    make(map[string]bool),
		required: make(map[string]bool),
	}

	starred := false

	for _, param := range def.Params {
		switch actual := param.(type) {
		case *syntax.Ident:
			signature._Add(actual.Name, starred)
			signature.required[actual.Name] = true

		case *syntax.BinaryExpr:
			defaulted, ok := actual.X.(*syntax.Ident)
			if ok {
				signature._Add(defaulted.Name, starred)
			}

		case *syntax.UnaryExpr:
			if actual.Op == syntax.STARSTAR {
				signature.keywords = true

				continue
			}

			// A bare * only ends what a call may fill by position. A *args
			// also takes any number more.
			signature.rest = actual.X != nil
			starred = true
		}
	}

	return signature
}

// _Add records a parameter a keyword may fill, and a call may fill by
// position unless it comes after a * or a *args.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func (s *_Signature) _Add(name string, starred bool) {
	s.named[name] = true

	if !starred {
		s.positional = append(s.positional, name)
	}
}

// _Takes reports whether call's arguments bind to this signature as Starlark
// binds them: by position first, then by name.
//
// Revisions:
//   - 2026-10-02 01:23: initial creation
func (s *_Signature) _Takes(call *syntax.CallExpr) bool {
	filled := make(map[string]bool)
	passed := 0

	for _, arg := range call.Args {
		keyword, ok := arg.(*syntax.BinaryExpr)
		if !ok {
			passed++

			continue
		}

		name, ok := keyword.X.(*syntax.Ident)
		if !ok || !s._Fill(name.Name, filled) {
			return false
		}
	}

	if passed > len(s.positional) && !s.rest {
		return false
	}

	for _, name := range s.positional[:min(passed, len(s.positional))] {
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
func (s *_Signature) _Fill(name string, filled map[string]bool) bool {
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
