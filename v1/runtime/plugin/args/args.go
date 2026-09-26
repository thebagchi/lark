// Package args gives a script the arguments its run was handed. Importing it
// is what enables it.
//
// A script declares one at module level and binds it to a name of its own
// choosing:
//
//	host = arg("host", "localhost")
//
// The two names need not agree, so a graph carries both. What a run supplies
// travels on the context and is bound by whoever initialises the module, which
// is why this reads the thread rather than holding anything itself.
package args

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/graph"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

var (
	// ErrNotSupplied is returned for an argument a script declared with no
	// default and the run did not supply.
	ErrNotSupplied = errors.New("argument not supplied and has no default")

	// ErrNotDeclaring is returned for an arg() call somewhere a declaration
	// cannot be: inside a function body rather than at module level.
	//
	// Without it such a call is the worst of the outcomes available. It takes
	// its default in silence, because the thread running a body is not the
	// thread a run bound its arguments on, and deriving a graph does not carry
	// it either - so a script that looks like it reads an argument reads
	// nothing, twice over, invisibly.
	ErrNotDeclaring = errors.New("arg declares a module-level name")

	// ErrNotValue is returned for a supplied value this cannot hand a script,
	// which is a google.protobuf.Value nobody filled in.
	ErrNotValue = errors.New("argument is not a value")
)

const (
	// NAME is what this plugin is called when a conflict has to name it.
	NAME = "args"

	// DEFAULT is the second parameter, so a script may pass it either way
	// round.
	DEFAULT = "default"

	// SUPPLIED is the first, which is the name a run supplies it under rather
	// than the name the script binds.
	SUPPLIED = "name"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func init() {
	plugin.Register(new(_Args))
}

// _Args is the plugin. Empty: what a run supplied belongs to that run and is
// found on the thread, so there is nothing here to hold.
type _Args struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func (a *_Args) Name() string {
	return NAME
}

// Values returns the declaration builtin.
//
// The name is the graph package's, because that package reads a declaration
// out of a syntax tree and this one writes the builtin the tree names. One
// spelling, in the package that cannot import the other.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func (a *_Args) Values() starlark.StringDict {
	return starlark.StringDict{
		graph.ARG: starlark.NewBuiltin(graph.ARG, _Declare),
	}
}

// _Declare is the builtin: the value the run supplied under name, or the
// default the script stated.
//
// Returns ErrNotSupplied when neither exists, so a script declaring an
// argument it cannot do without fails at the line that declares it rather than
// somewhere later with None in its hands. Returns ErrNotDeclaring when it is
// called anywhere but a module's top level.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Declare(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		name     string
		fallback starlark.Value
	)

	err := starlark.UnpackArgs(
		fn.Name(),
		args,
		kwargs,
		SUPPLIED,
		&name,
		DEFAULT+"?",
		&fallback,
	)
	if err != nil {
		return nil, err
	}

	supplied, declaring := artifact.Supplied(thread)
	if !declaring {
		return nil, fmt.Errorf("%s: %w", name, ErrNotDeclaring)
	}

	value, found := supplied[name]
	if found {
		return _Starlark(value)
	}

	if fallback == nil {
		return nil, fmt.Errorf("%s: %w", name, ErrNotSupplied)
	}

	return fallback, nil
}

// _Starlark is a supplied value as a script sees it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Starlark(value *structpb.Value) (starlark.Value, error) {
	switch held := value.GetKind().(type) {
	case *structpb.Value_NullValue:
		return starlark.None, nil

	case *structpb.Value_NumberValue:
		return _Number(held.NumberValue), nil

	case *structpb.Value_StringValue:
		return starlark.String(held.StringValue), nil

	case *structpb.Value_BoolValue:
		return starlark.Bool(held.BoolValue), nil

	case *structpb.Value_ListValue:
		return _List(held.ListValue)

	case *structpb.Value_StructValue:
		return _Dict(held.StructValue)
	}

	return nil, fmt.Errorf("%T: %w", value.GetKind(), ErrNotValue)
}

// _Number is a supplied number as a script sees it.
//
// A whole number arrives as an int, because JSON has one number type and
// Starlark has two: a script indexing a list or counting a repeat with a float
// gets an error from Starlark rather than the count its caller passed.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Number(number float64) starlark.Value {
	whole := int64(number)
	if float64(whole) != number {
		return starlark.Float(number)
	}

	return starlark.MakeInt64(whole)
}

// _List is a supplied array as a script sees it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _List(values *structpb.ListValue) (starlark.Value, error) {
	held := make([]starlark.Value, 0, len(values.GetValues()))

	for _, value := range values.GetValues() {
		converted, err := _Starlark(value)
		if err != nil {
			return nil, err
		}

		held = append(held, converted)
	}

	return starlark.NewList(held), nil
}

// _Dict is a supplied object as a script sees it.
//
// Revisions:
//   - 2026-09-22 22:24: initial creation
func _Dict(fields *structpb.Struct) (starlark.Value, error) {
	held := starlark.NewDict(len(fields.GetFields()))

	for key, value := range fields.GetFields() {
		converted, err := _Starlark(value)
		if err != nil {
			return nil, err
		}

		err = held.SetKey(starlark.String(key), converted)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
	}

	return held, nil
}
