package artifact

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"unsafe"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Seen is the identity of a container the walk is inside: where its data
// starts, how many elements it has, and its Go type. A slice and its prefix
// window start at the same address, so the address alone would call a window a
// cycle; a window holds fewer elements, so it is another container. Maps and
// pointers have no length: size is 0.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Seen struct {
	at   unsafe.Pointer
	size int
	kind reflect.Type
}

// _Walk is the state of one conversion of one argument: the name it is
// supplied under, for an error.
//
// The containers the walk is inside, by identity, travel as a parameter of each
// step: a cycle and the depth are found from them. A step that goes into a
// container passes its callee a longer path and keeps its own, so there is
// nothing to leave. The path lives in an array of _Converted, and a Go slice
// stored in a struct would put that array on the heap: a value nested up to
// common.INLINE deep costs the walk no allocation.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Walk struct {
	key string
}

// _Converted is args with every typed slice, typed map, named basic type and
// pointer in it replaced by the plain value structpb.NewValue takes: a list of
// []any, a struct of map[string]any, a basic value.
//
// A map or slice that holds nothing to change is returned as it is, so a call
// with plain arguments allocates nothing here. Only a container on the way to a
// changed value is copied, and the caller's own is never written. A value JSON
// cannot hold, a cycle, and nesting past common.DEPTH are errors that name the
// argument.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
func _Converted(args map[string]any) (map[string]any, error) {
	var (
		made   map[string]any
		walk   _Walk
		inline [common.INLINE]_Seen
	)

	path := inline[:0]

	for key, given := range args {
		walk.key = key

		held, changed, err := walk._Convert(
			path,
			given,
		)
		if err != nil {
			return nil, err
		}

		if !changed {
			continue
		}

		if made == nil {
			made = maps.Clone(args)
		}

		made[key] = held
	}

	if made == nil {
		return args, nil
	}

	return made, nil
}

// _Convert returns given as a value structpb.NewValue takes, and whether it
// differs from given.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-10 11:51: refuses NaN and the infinities (C117, B244, Q297)
func (w *_Walk) _Convert(
	path []_Seen,
	given any,
) (any, bool, error) {
	switch held := given.(type) {
	case nil, bool, string, []byte,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return given, false, nil

	case json.Number:
		// A text that is no float is left to structpb, which refuses it.
		number, err := strconv.ParseFloat(
			string(held),
			64,
		)
		if err != nil {
			return given, false, nil
		}

		return given, false, w._Finite(number)

	case float32:
		return given, false, w._Finite(float64(held))

	case float64:
		return given, false, w._Finite(held)

	case map[string]any:
		got, changed, err := w._PlainMap(
			path,
			held,
		)
		if !changed {
			return given, false, err
		}

		return got, true, err

	case []any:
		got, changed, err := w._PlainList(
			path,
			held,
		)
		if !changed {
			return given, false, err
		}

		return got, true, err
	}

	held, err := w._Reflected(
		path,
		reflect.ValueOf(given),
	)

	return held, true, err
}

// _PlainMap converts the entries of held, copying it on the first that changes.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
func (w *_Walk) _PlainMap(
	path []_Seen,
	held map[string]any,
) (map[string]any, bool, error) {
	if len(held) == 0 {
		return held, false, nil
	}

	path, err := w._Enter(
		path,
		&_Seen{
			at: reflect.ValueOf(held).UnsafePointer(),
		},
	)
	if err != nil {
		return nil, false, err
	}

	var made map[string]any

	for key, entry := range held {
		got, changed, err := w._Convert(
			path,
			entry,
		)
		if err != nil {
			return nil, false, err
		}

		if !changed {
			continue
		}

		if made == nil {
			made = maps.Clone(held)
		}

		made[key] = got
	}

	if made == nil {
		return held, false, nil
	}

	return made, true, nil
}

// _PlainList converts the elements of held, copying it on the first that
// changes.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
func (w *_Walk) _PlainList(
	path []_Seen,
	held []any,
) ([]any, bool, error) {
	if len(held) == 0 {
		return held, false, nil
	}

	path, err := w._Enter(
		path,
		&_Seen{
			at:   unsafe.Pointer(unsafe.SliceData(held)),
			size: len(held),
		},
	)
	if err != nil {
		return nil, false, err
	}

	var made []any

	for index, entry := range held {
		got, changed, err := w._Convert(
			path,
			entry,
		)
		if err != nil {
			return nil, false, err
		}

		if !changed {
			continue
		}

		if made == nil {
			made = slices.Clone(held)
		}

		made[index] = got
	}

	if made == nil {
		return held, false, nil
	}

	return made, true, nil
}

// _Reflected converts a value of any other type by its kind: a basic kind to its
// value, a pointer or an interface to what it holds (nil to nil), a slice or an
// array to a list, a map with a string key kind to a struct. Any other kind,
// and a map with another key kind, is refused.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-10 11:51: refuses NaN and the infinities (C117, B244, Q297)
func (w *_Walk) _Reflected(
	path []_Seen,
	value reflect.Value,
) (any, error) {
	switch value.Kind() {
	case reflect.Invalid:
		return nil, nil

	case reflect.Bool:
		return value.Bool(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64:
		return value.Uint(), nil

	case reflect.Float32, reflect.Float64:
		return value.Float(), w._Finite(value.Float())

	case reflect.String:
		return value.String(), nil

	case reflect.Interface:
		if value.IsNil() {
			return nil, nil
		}

		return w._Reflected(
			path,
			value.Elem(),
		)

	case reflect.Pointer:
		return w._Pointed(
			path,
			value,
		)

	case reflect.Slice, reflect.Array:
		return w._Listed(
			path,
			value,
		)

	case reflect.Map:
		return w._Mapped(
			path,
			value,
		)
	}

	return nil, w._Refused(value.Type())
}

// _Pointed converts what a pointer points to.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
func (w *_Walk) _Pointed(
	path []_Seen,
	value reflect.Value,
) (any, error) {
	if value.IsNil() {
		return nil, nil
	}

	path, err := w._Enter(
		path,
		&_Seen{
			at:   value.UnsafePointer(),
			kind: value.Type(),
		},
	)
	if err != nil {
		return nil, err
	}

	return w._Reflected(
		path,
		value.Elem(),
	)
}

// _Listed converts a slice or an array to a []any. A slice of bytes stays a
// []byte, which structpb.NewValue writes as base64 text, as it does today.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
func (w *_Walk) _Listed(
	path []_Seen,
	value reflect.Value,
) (any, error) {
	if value.Kind() == reflect.Slice &&
		value.Type().Elem().Kind() == reflect.Uint8 {
		return value.Bytes(), nil
	}

	list := make(
		[]any,
		0,
		value.Len(),
	)

	if value.Kind() == reflect.Slice && value.Len() > 0 {
		var err error

		path, err = w._Enter(
			path,
			&_Seen{
				at:   value.UnsafePointer(),
				size: value.Len(),
				kind: value.Type(),
			},
		)
		if err != nil {
			return nil, err
		}
	}

	for index := range value.Len() {
		got, err := w._Reflected(
			path,
			value.Index(index),
		)
		if err != nil {
			return nil, err
		}

		list = append(
			list,
			got,
		)
	}

	return list, nil
}

// _Mapped converts a map with a string key kind to a map[string]any.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
func (w *_Walk) _Mapped(
	path []_Seen,
	value reflect.Value,
) (any, error) {
	if value.Type().Key().Kind() != reflect.String {
		return nil, w._Refused(value.Type())
	}

	made := make(
		map[string]any,
		value.Len(),
	)

	if value.Len() > 0 {
		var err error

		path, err = w._Enter(
			path,
			&_Seen{
				at:   value.UnsafePointer(),
				size: value.Len(),
				kind: value.Type(),
			},
		)
		if err != nil {
			return nil, err
		}
	}

	for each := value.MapRange(); each.Next(); {
		got, err := w._Reflected(
			path,
			each.Value(),
		)
		if err != nil {
			return nil, err
		}

		made[each.Key().String()] = got
	}

	return made, nil
}

// _Enter returns path with at, the container the walk goes into, added. A
// container already on
// the path is trail.ErrCyclic, and one past common.DEPTH is
// trail.ErrDeep.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
//   - 2026-11-05 17:30: identity has length and type, so a window is no
//     cycle (C85 fix round 1)
//   - 2026-11-10 17:15: uses ErrCyclic and ErrDeep, formerly ErrSelfContaining and ErrTooDeep
//     (C121, Q349)
func (w *_Walk) _Enter(
	path []_Seen,
	at *_Seen,
) ([]_Seen, error) {
	if slices.Contains(
		path,
		*at,
	) {
		return path, w._Failed(trail.ErrCyclic)
	}

	if len(path) >= common.DEPTH {
		return path, w._Failed(trail.ErrDeep)
	}

	return append(
		path,
		*at,
	), nil
}

// _Refused is the error for a Go type JSON cannot hold. Both the argument name
// and the type name are quoted and cut (Q4): the first is the host's, the
// second can be as long as a type a host builds.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
func (w *_Walk) _Refused(kind reflect.Type) error {
	return fmt.Errorf(
		"argument %s: Go type %s is not a JSON value; "+
			"use a basic type, a slice, an array or a map with string keys "+
			"of them, or []any and map[string]any",
		unpack.QuotePrefix(w.key),
		unpack.QuotePrefix(kind.String()),
	)
}

// _Failed puts the argument's name in front of err, which stays the cause.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (C85)
func (w *_Walk) _Failed(err error) error {
	return fmt.Errorf(
		"argument %s: %w",
		unpack.QuotePrefix(w.key),
		err,
	)
}

// _Finite is nil for a number JSON can hold, and a refusal for NaN and the
// infinities, which it cannot (Q297).
//
// The text spells the number with %v, which gives only NaN, +Inf or -Inf here.
//
// Revisions:
//   - 2026-11-10 11:51: initial creation (C117, B244, Q297)
func (w *_Walk) _Finite(number float64) error {
	if !math.IsNaN(number) && !math.IsInf(
		number,
		0,
	) {
		return nil
	}

	return w._Failed(fmt.Errorf(
		"%v is not a JSON number (JSON has no NaN or infinity)",
		number,
	))
}
