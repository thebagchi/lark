package jsonpath

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	NAME     = "jsonpath"
	PATCH    = "patch_json"
	FIND     = "find_key"
	EXTRACT  = "extract_json"
	MATCH    = "match_json"
	LENGTH   = "len_json"
	NO_VALUE = 0
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 01:12: initial creation
func init() {
	plugin.Register(&_Plugin{})
}

// _Plugin is the plugin. Empty: every call works on the document it is given,
// so there is nothing to hold.
type _Plugin struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 01:12: initial creation
func (p *_Plugin) Name() string {
	return NAME
}

// Values returns the five names this plugin supplies.
//
// They are flat rather than members of a module, because that is how §10.7 of
// the brief spells them and how the scripts written against it call them.
//
// Revisions:
//   - 2026-09-20 01:13: initial creation
func (p *_Plugin) Values() starlark.StringDict {
	return starlark.StringDict{
		PATCH:   starlark.NewBuiltin(PATCH, _PatchJSON),
		FIND:    starlark.NewBuiltin(FIND, _FindKey),
		EXTRACT: starlark.NewBuiltin(EXTRACT, _ExtractJSON),
		MATCH:   starlark.NewBuiltin(MATCH, _MatchJSON),
		LENGTH:  starlark.NewBuiltin(LENGTH, _LenJSON),
	}
}

// _PatchJSON applies a list of RFC 6902 operations and returns a new document.
//
// The input is never mutated. Operations apply in order, and the first failure
// stops the patch - a half-applied document is not a document anybody asked
// for.
//
// Revisions:
//   - 2026-09-20 01:14: initial creation
func _PatchJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		doc   starlark.Value
		patch *starlark.List
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &doc, &patch)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	for index := range patch.Len() {
		op, ok := patch.Index(index).(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf(
				"%s: operation %d is %s: %w",
				fn.Name(),
				index,
				patch.Index(index).Type(),
				ErrOperation,
			)
		}

		doc, err = _Patch(doc, op)
		if err != nil {
			return nil, fmt.Errorf("%s: operation %d: %w", fn.Name(), index, err)
		}
	}

	return doc, nil
}

// _ExtractJSON returns the value at a pointer, or None when any step is
// missing.
//
// None rather than an error, because a script asking whether an optional field
// is present is the ordinary case. A malformed pointer is still an error: that
// is the script being wrong, not the document.
//
// Revisions:
//   - 2026-09-20 01:15: initial creation
func _ExtractJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, steps, err := _Target(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	value, err := _Walk(doc, steps)
	if err != nil {
		return _Absent(fn, err, starlark.None)
	}

	return value, nil
}

// _MatchJSON reports whether the value at a pointer deep-equals a value.
//
// A missing path is False rather than an error, which is RFC 6902's test
// semantics read as a question instead of an assertion.
//
// Revisions:
//   - 2026-09-20 01:16: initial creation
func _MatchJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		doc     starlark.Value
		pointer string
		want    starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 3, &doc, &pointer, &want)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	steps, err := _Steps(pointer)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	got, err := _Walk(doc, steps)
	if err != nil {
		return _Absent(fn, err, starlark.False)
	}

	same, err := _Equal(got, want)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.Bool(same), nil
}

// _LenJSON returns the length of what a pointer names, or 0 when it is
// missing.
//
// A value that exists but has no length is an error, because that is a script
// asking a question of the wrong thing - unlike a missing path, which is a
// document simply not having something.
//
// Revisions:
//   - 2026-09-20 01:17: initial creation
func _LenJSON(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	doc, steps, err := _Target(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	value, err := _Walk(doc, steps)
	if err != nil {
		return _Absent(fn, err, starlark.MakeInt(NO_VALUE))
	}

	measured, ok := value.(starlark.Sequence)
	if ok {
		return starlark.MakeInt(measured.Len()), nil
	}

	switch sized := value.(type) {
	case starlark.String:
		return starlark.MakeInt(sized.Len()), nil

	case *starlark.Dict:
		return starlark.MakeInt(sized.Len()), nil

	default:
		return nil, fmt.Errorf("%s: %s has no length: %w", fn.Name(), value.Type(), ErrKind)
	}
}

// _FindKey returns the first member named key, searching depth first, or None.
//
// There is no path argument: this is the search a script reaches for when it
// knows a name appears once and does not want to spell out where.
//
// Revisions:
//   - 2026-09-20 01:18: initial creation
func _FindKey(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		doc starlark.Value
		key string
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &doc, &key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	found, err := _Search(doc, key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	if found == nil {
		return starlark.None, nil
	}

	return found, nil
}

// _Search walks doc depth first for a member named key.
//
// Revisions:
//   - 2026-09-20 01:19: initial creation
func _Search(doc starlark.Value, key string) (starlark.Value, error) {
	switch container := doc.(type) {
	case *starlark.Dict:
		value, present, err := container.Get(starlark.String(key))
		if err != nil {
			return nil, err
		}

		if present {
			return value, nil
		}

		for _, item := range container.Items() {
			found, err := _Search(item[1], key)
			if err != nil || found != nil {
				return found, err
			}
		}

	case *starlark.List:
		for index := range container.Len() {
			found, err := _Search(container.Index(index), key)
			if err != nil || found != nil {
				return found, err
			}
		}
	}

	return nil, nil
}

// _Absent turns a walk failure into the answer a query gives for something
// that is not there, or passes it on when it is not that kind of failure.
//
// A document not having something is an answer: None, False, zero. A pointer
// that is not a pointer is the script being wrong, and must not arrive looking
// like an empty document - which is exactly what swallowing every error did
// until a test asked for "/a/01" and was told None.
//
// Revisions:
//   - 2026-09-20 01:35: initial creation
func _Absent(fn *starlark.Builtin, err error, answer starlark.Value) (starlark.Value, error) {
	if errors.Is(err, ErrMissing) || errors.Is(err, ErrKind) {
		return answer, nil
	}

	return nil, fmt.Errorf("%s: %w", fn.Name(), err)
}

// _Target reads the document and pointer two of these builtins share.
//
// Revisions:
//   - 2026-09-20 01:20: initial creation
func _Target(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, []string, error) {
	var (
		doc     starlark.Value
		pointer string
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &doc, &pointer)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	steps, err := _Steps(pointer)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return doc, steps, nil
}
