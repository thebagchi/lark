package remote

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"
)

// EXACT is the largest integer a float64 holds exactly, and so the largest one
// that can cross without changing.
const EXACT = 1 << 53

// ErrValue is returned for a value the wire cannot name.
//
// Separate from ErrArgument, which is the question asked before marshalling.
// This one is the marshalling itself failing, which after IsData has agreed
// means the two notions of data have drifted apart - so it names a defect
// here rather than a mistake in the script.
var ErrValue = errors.New("cannot be put on the wire")

// _Value is a Starlark value as protobuf names it.
//
// Only what deep.IsData already agreed to: None, bools, numbers, strings,
// bytes and the containers of those. Anything else is this package and IsData
// disagreeing, which is a defect rather than a script's mistake.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Value(given starlark.Value) (*structpb.Value, error) {
	switch held := given.(type) {
	case starlark.NoneType:
		return structpb.NewNullValue(), nil

	case starlark.Bool:
		return structpb.NewBoolValue(bool(held)), nil

	case starlark.Int:
		// A protobuf number is a float64, which holds integers exactly only up
		// to EXACT. Past that an integer would cross as a different number
		// than it left, and a plugin would be handed a quiet approximation -
		// the kind of defect that surfaces as arithmetic being wrong somewhere
		// else entirely.
		//
		// Compared as integers. Comparing the float round trip was the first
		// attempt and it cannot work: the two numbers being told apart are the
		// same float64, which is the whole problem.
		exact, ok := held.Int64()
		if !ok || exact > EXACT || exact < -EXACT {
			return nil, fmt.Errorf("%s does not fit a wire number: %w", held, ErrValue)
		}

		return structpb.NewNumberValue(float64(exact)), nil

	case starlark.Float:
		return structpb.NewNumberValue(float64(held)), nil

	case starlark.String:
		return structpb.NewStringValue(string(held)), nil

	case starlark.Bytes:
		return structpb.NewStringValue(string(held)), nil

	case *starlark.List:
		return _List(held.Len(), held.Index)

	case starlark.Tuple:
		return _List(held.Len(), held.Index)

	case *starlark.Dict:
		return _Dict(held)
	}

	return nil, fmt.Errorf("%s: %w", given.Type(), ErrValue)
}

// _List is a sequence as a protobuf list.
//
// Takes the length and the accessor rather than the value, because a list and
// a tuple answer the same two questions and do not share an interface that
// says so.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _List(length int, at func(int) starlark.Value) (*structpb.Value, error) {
	held := make([]*structpb.Value, 0, length)

	for index := range length {
		part, err := _Value(at(index))
		if err != nil {
			return nil, err
		}

		held = append(held, part)
	}

	return structpb.NewListValue(&structpb.ListValue{Values: held}), nil
}

// _Dict is a dictionary as a protobuf struct.
//
// A protobuf struct keys by string, so a dictionary keyed by anything else
// cannot cross. That is a narrower rule than IsData, which allows a number as
// a key, and it is the one place the wire is stricter than the store.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Dict(given *starlark.Dict) (*structpb.Value, error) {
	held := map[string]*structpb.Value{}

	for _, pair := range given.Items() {
		key, ok := starlark.AsString(pair[0])
		if !ok {
			return nil, fmt.Errorf("a key of %s: %w", pair[0].Type(), ErrValue)
		}

		part, err := _Value(pair[1])
		if err != nil {
			return nil, err
		}

		held[key] = part
	}

	return structpb.NewStructValue(&structpb.Struct{Fields: held}), nil
}

// _Starlark is a protobuf value as Starlark sees it.
//
// Everything protobuf can carry is data, so this cannot fail on the kind of
// thing it is given - unlike the direction above, where Starlark is the richer
// of the two.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
func _Starlark(given *structpb.Value) (starlark.Value, error) {
	switch held := given.GetKind().(type) {
	case *structpb.Value_NullValue, nil:
		return starlark.None, nil

	case *structpb.Value_BoolValue:
		return starlark.Bool(held.BoolValue), nil

	case *structpb.Value_NumberValue:
		return starlark.Float(held.NumberValue), nil

	case *structpb.Value_StringValue:
		return starlark.String(held.StringValue), nil

	case *structpb.Value_ListValue:
		parts := held.ListValue.GetValues()
		into := make([]starlark.Value, 0, len(parts))

		for _, part := range parts {
			one, err := _Starlark(part)
			if err != nil {
				return nil, err
			}

			into = append(into, one)
		}

		return starlark.NewList(into), nil

	case *structpb.Value_StructValue:
		into := starlark.NewDict(len(held.StructValue.GetFields()))

		for key, part := range held.StructValue.GetFields() {
			one, err := _Starlark(part)
			if err != nil {
				return nil, err
			}

			err = into.SetKey(starlark.String(key), one)
			if err != nil {
				return nil, err
			}
		}

		return into, nil
	}

	return nil, fmt.Errorf("%T: %w", given.GetKind(), ErrValue)
}
