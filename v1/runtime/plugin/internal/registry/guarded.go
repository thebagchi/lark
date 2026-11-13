// Package registry is internal, not public: it holds the runtime's own
// plugins, the guard around their builtins, and the merge into one
// environment.
//
// Package registry holds the runtime's own plugins, the guard around every
// builtin they supply, and the merge of plugins into one environment. Only the
// package plugin imports it.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation, from plugin.go and guarded.go (C91w1)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package registry

import (
	"fmt"
	"maps"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Guarded is value with every builtin it holds wrapped by Checked: value
// itself when it is a builtin, a copy of it when it is a module whose members
// are, and value as it is for anything else.
//
// A module is copied rather than changed, because Values returns what a plugin
// may keep, and wrapping its members in place would wrap them again on the next
// compile.
//
// A value that is nil, or a nil pointer, map, slice, function or channel held in
// the interface, is refused with an error that names it by name, a module member
// as "module.member". Freeze, the wrapper and a script's call would each
// dereference it (B248, B249, B250, B267). A nil tuple is not one of them: it is
// the empty tuple, and Freeze and a call read it without a dereference. The error
// has no sentinel (Q180).
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 01:00: now uses Checked, were _Checked (C120a, Q314)
//   - 2026-11-10 11:54: refuses a nil value and a typed-nil one, which a module member
//     may be too (C117, B248, B249, B250, Q65, Q206)
//   - 2026-11-11 00:38: refuses a nil of any kind, with nilcheck.Absent; a nil tuple stays
//     (C122, B267, B268)
func _Guarded(
	name string,
	value starlark.Value,
) (starlark.Value, error) {
	_, tuple := value.(starlark.Tuple)
	if !tuple && nilcheck.Absent(value) {
		return nil, fmt.Errorf(
			"plugin name %s is a nil value (give it a value or leave the name out)",
			unpack.QuotePrefix(name),
		)
	}

	switch held := value.(type) {
	case *starlark.Builtin:
		return Checked(held), nil

	case *starlarkstruct.Module:
		members := make(
			starlark.StringDict,
			len(held.Members),
		)

		for member, each := range held.Members {
			guarded, err := _Guarded(
				name+"."+member,
				each,
			)
			if err != nil {
				return nil, err
			}

			members[member] = guarded
		}

		return &starlarkstruct.Module{
			Name:    held.Name,
			Members: members,
		}, nil
	}

	return value, nil
}

// NewBuiltin is starlark.NewBuiltin with a check of the arguments in front of
// fn, and is the one wrapper every guarded builtin goes through.
//
// The body fn has the shape starlark.NewBuiltin takes: the thread, the builtin
// being called, its positional arguments and its keyword pairs.
//
// A host plugin author should build each builtin of a plugin with it, in place
// of starlark.NewBuiltin, whenever fn reads its arguments with
// starlark.UnpackArgs or starlark.UnpackPositionalArgs and a Go caller may call
// the builtin directly. The runtime's own plugins are wrapped already, by
// Register, and a plugin handed to a compile is wrapped by Environment, so a
// host that does nothing is covered too; use NewBuiltin to be explicit, or
// where a builtin is built later than Values, as a method bound to a value.
//
// go.starlark.net's UnpackArgs and UnpackPositionalArgs panic on a nil Value
// (they call Type on it) and on a keyword pair that is short or whose name is
// not a string (they index and assert without a check). A script cannot build
// either, but a host calling a builtin from Go can. Nearly every plugin
// unpacks this way, so the check stands here once, in front of all of them,
// and not in each.
//
// fn is called with the builtin it was given, so a receiver bound later with
// BindReceiver reaches it, and nothing is allocated per call. It checks and
// does not recover: the scheduler already recovers the panics of a script run,
// and a second recover on that path would only hide where the first one
// stopped. The error text holds the builtin's name and a position, and never
// the argument.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93), as the unexported _Checked
//   - 2026-11-04 22:35: exported, takes the body, so buf's method tables use it
//     (Q134)
//   - 2026-11-04 22:35: documented for host plugin authors, public by decision (Q143)
//   - 2026-11-07 18:16: moved to package registry; BuiltinFunc dropped, the func type spelled out
//     (Q222, Q223, C91w1)
func NewBuiltin(
	name string,
	fn func(
		thread *starlark.Thread,
		self *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error),
) *starlark.Builtin {
	return starlark.NewBuiltin(
		name,
		func(
			thread *starlark.Thread,
			self *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			err := _Malformed(
				name,
				args,
				kwargs,
			)
			if err != nil {
				return nil, err
			}

			return fn(
				thread,
				self,
				args,
				kwargs,
			)
		},
	)
}

// Checked is builtin behind the check of NewBuiltin, for a builtin whose body
// is not at hand.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-04 22:35: goes through NewBuiltin, the one wrapper (Q134)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 00:25: renamed from _Checked, as a test in tests/ needs it (C120a, Q314)
func Checked(builtin *starlark.Builtin) *starlark.Builtin {
	return NewBuiltin(
		shared.Called(builtin),
		func(
			thread *starlark.Thread,
			_ *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return builtin.CallInternal(
				thread,
				args,
				kwargs,
			)
		},
	)
}

// _Guarding is a plugin whose Values come already checked, so that a Go caller
// of Values and Environment both get builtins that refuse a nil Value.
//
// A registered plugin is cached: its names are built and wrapped once, on the
// first call, and every later call reuses them. A plugin a host hands to a
// compile is not: its names may change between calls, as a remote plugin's do.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
type _Guarding struct {
	inner  common.Plugin
	cached bool
	once   sync.Once
	held   starlark.StringDict
	broken error
}

// Name is the name of the plugin it holds.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
func (g *_Guarding) Name() string {
	return g.inner.Name()
}

// Values is what the plugin it holds supplies, each builtin checked.
//
// The dict is the caller's own, a copy; the builtins and modules in it are
// shared by every caller, and frozen.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-04 22:35: copies the cached dict, so a caller that changes it harms no one
//     (Q142)
//   - 2026-11-05 16:55: panics, as the plugin did, when its Values panicked; Environment
//     reads _Shared and returns the error instead (B123, C96)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
func (g *_Guarding) Values() starlark.StringDict {
	shared, err := g._Shared()
	if err != nil {
		panic(err)
	}

	return maps.Clone(shared)
}

// Check passes the tree to the plugin it holds when that is a Checking, so
// wrapping hides no refusal. A panic of the plugin's Check is returned as an
// error that names the plugin and wraps the panic.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-05 17:40: contains a panic of the plugin's Check, through _Called (B123, C96
//     fix round 1)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
func (g *_Guarding) Check(tree *syntax.File) error {
	checker, ok := g.inner.(common.Checking)
	if !ok {
		return nil
	}

	var refusal error

	err := g._Called(
		"Check",
		func() {
			refusal = checker.Check(tree)
		},
	)
	if err != nil {
		return err
	}

	return refusal
}

// _Called runs call, which is one call into the plugin it holds, and returns
// the panic of that call as an error that names the plugin and the method and
// wraps the recovered panic (guard.Contained, Q113). It is the one place a
// panic of host plugin code is contained: Values and Check both go through it.
//
// No sentinel wraps the error (Q179 and Q180). guard formats a panic value that
// is not an error with %v, so only an error panic value stays in the chain.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation, from _Built (B123, C96 fix round 1)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
func (g *_Guarding) _Called(
	method string,
	call func(),
) error {
	raised := guard.Contained(call)
	if raised == nil {
		return nil
	}

	return fmt.Errorf(
		"%s: plugin %s panicked: %w",
		g._Label(),
		method,
		raised,
	)
}

// _Label is the name of the plugin it holds, or "unnamed" when Name panics, so
// that the error for a panic can still name something.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation (B123, C96 fix round 1)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 16:05: cuts the name it returns to common.QUOTE_LIMIT bytes (C117, B239, Q4)
func (g *_Guarding) _Label() string {
	var (
		label  = "unnamed"
		raised = guard.Contained(func() {
			label = g.inner.Name()
		})
	)
	if raised != nil {
		return "unnamed"
	}

	return unpack.ShortText(label)
}

// _Shared is the dict of the plugin, its builtins checked: built once and
// frozen when the plugin is cached, built afresh otherwise. The caller reads it
// and never changes it.
//
// sync.Once makes the first build safe for compiles that start together. When
// the plugin's Values panics, or a name is nil, or a member's Freeze panics,
// the error is kept with the dict, so a cached plugin does not call Values
// again and every later call returns the same error, not an empty dict with no
// error (B249).
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-05 16:55: returns the error of a panicking Values (B123, C96)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 11:54: contains a panic of Freeze and keeps it as the error, so a
//     failed first build is not a success on the next call (C117, B249)
func (g *_Guarding) _Shared() (starlark.StringDict, error) {
	if !g.cached {
		return g._Built()
	}

	g.once.Do(func() {
		g.held, g.broken = g._Built()
		if g.broken != nil {
			return
		}

		g.broken = g._Called(
			"Freeze",
			g.held.Freeze,
		)
		if g.broken != nil {
			g.held = starlark.StringDict{}
		}
	})

	return g.held, g.broken
}

// _Built is the names the plugin it holds supplies, each builtin checked. A
// plugin whose Values panics gives no names and an error that names the plugin
// and wraps the panic (guard.Contained, Q113).
//
// Revisions:
//   - 2026-11-04 22:35: initial creation, from Values (Q142)
//   - 2026-11-05 16:55: contains a panic of the plugin's Values (B123, C96)
//   - 2026-11-05 17:40: contains it through _Called (B123, C96 fix round 1)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 11:54: a nil name or module member is an error that names the
//     plugin (C117, B248, B249, B250)
func (g *_Guarding) _Built() (starlark.StringDict, error) {
	var held starlark.StringDict

	err := g._Called(
		"Values",
		func() {
			held = g.inner.Values()
		},
	)
	if err != nil {
		return starlark.StringDict{}, err
	}

	values := make(
		starlark.StringDict,
		len(held),
	)

	for name, value := range held {
		guarded, err := _Guarded(
			name,
			value,
		)
		if err != nil {
			return starlark.StringDict{}, fmt.Errorf(
				"%s: %w",
				g._Label(),
				err,
			)
		}

		values[name] = guarded
	}

	return values, nil
}

// Wrapped is plugin as a _Guarding, or plugin itself when it already is one,
// so a builtin is wrapped once however many times a plugin is passed through.
//
// The wrapper does not cache: a plugin that reaches here is a host's, whose
// names may change between compiles, as a remote plugin's do. Only Register
// makes a cached one.
//
// Revisions:
//   - 2026-11-04 22:40: initial creation (Q134)
//   - 2026-11-04 22:40: takes no flag; caching is decided in Register alone (Q147)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
//   - 2026-11-10 00:25: renamed from _Wrapped, as a test in tests/ needs it (C120a, Q314)
func Wrapped(plugin common.Plugin) *_Guarding {
	done, ok := plugin.(*_Guarding)
	if ok {
		return done
	}

	return &_Guarding{
		inner: plugin,
	}
}

// _Malformed is the error for a call that would panic go.starlark.net's
// unpacking, or nil when the call is well formed.
//
// A keyword pair needs two elements, a string name first and a value that is
// not nil. An argument is never nil.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-07 18:16: moved to package registry (Q222) (C91w1)
func _Malformed(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) error {
	for i, arg := range args {
		if arg == nil {
			return fmt.Errorf(
				"%s: argument %d is nil",
				who,
				i+1,
			)
		}
	}

	// PAIR_LEN is the length of a keyword argument: a name and a value.
	const PAIR_LEN = 2

	for i, pair := range kwargs {
		if len(pair) < PAIR_LEN {
			return fmt.Errorf(
				"%s: keyword argument %d is not a name and a value",
				who,
				i+1,
			)
		}

		if _, ok := pair[0].(starlark.String); !ok {
			return fmt.Errorf(
				"%s: keyword argument %d has no string name",
				who,
				i+1,
			)
		}

		if pair[1] == nil {
			return fmt.Errorf(
				"%s: keyword argument %d is nil",
				who,
				i+1,
			)
		}
	}

	return nil
}
