// Package wire is internal, not public: it converts a Starlark value to a
// protobuf value, for the remote host and for runtime.Export.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation, moved from package host (C73, Q257)
package wire

import (
	"fmt"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// EXACT is the largest integer a float64 holds exactly, and so the largest one
// that can cross without changing.
//
// Revisions:
//   - 2026-09-25 06:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
const (
	EXACT = 1 << 53
)

// Value is a Starlark value as protobuf names it.
//
// Only what deep.IsData already agreed to: None, bools, numbers, strings,
// bytes and the containers of those. Anything else is this package and IsData
// disagreeing, which is a defect rather than a script's mistake.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-11-02 12:28: walks with a trail: a container that holds itself is
//     trail.ErrSelfContaining, and a value nested past common.DEPTH is
//     trail.ErrTooDeep (C26)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host, exported as Value; a nil
//     value is ErrValue, not a panic (C73, Q257)
//   - 2026-11-10 11:46: a typed-nil list, dict or set is refused like a nil
//     value, not dereferenced (C117, B240, Q65, Q206)
func Value(
	path *common.Trail,
	given starlark.Value,
) (*structpb.Value, error) {
	err := trail.Tick(path)
	if err != nil {
		return nil, err
	}

	switch held := given.(type) {
	case starlark.NoneType:
		return structpb.NewNullValue(), nil

	case starlark.Bool:
		return structpb.NewBoolValue(bool(held)), nil

	case starlark.Int:
		return _Number(held)

	case starlark.Float:
		return structpb.NewNumberValue(float64(held)), nil

	case starlark.String:
		return structpb.NewStringValue(string(held)), nil

	case starlark.Bytes:
		return structpb.NewStringValue(string(held)), nil

	case *starlark.List:
		return _Listed(
			path,
			held,
		)

	case starlark.Tuple:
		return _List(
			path,
			nil,
			held.Len(),
			held.Index,
		)

	case *starlark.Dict:
		return _Dict(
			path,
			held,
		)

	case *starlark.Set:
		return _Set(
			path,
			held,
		)
	}

	return nil, _Refused(given)
}

// _Refused is the error for a value the wire cannot name: a nil value, or one
// whose type has no protobuf form.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation, split from Value (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Refused(given starlark.Value) error {
	if given == nil {
		return fmt.Errorf(
			"a nil value: cannot be put on the wire: %w",
			unpack.ErrConvert,
		)
	}

	return fmt.Errorf(
		"%s: cannot be put on the wire: %w",
		given.Type(),
		unpack.ErrConvert,
	)
}

// _Listed is a list as a protobuf list, or the refusal of a nil list.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation, split from Value (C117, B240)
func _Listed(
	path *common.Trail,
	given *starlark.List,
) (*structpb.Value, error) {
	if given == nil {
		return nil, _Refused(nil)
	}

	return _List(
		path,
		given,
		given.Len(),
		given.Index,
	)
}

// _Set is a set as a protobuf list, in the order the set holds its members.
//
// deep.IsData accepts a set, so it must cross: a set had no case here, and the
// two notions of data had drifted apart (B004). The wire has no set, so it
// arrives as a list, as a tuple does.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host (C73, Q257)
//   - 2026-11-10 11:54: refuses a nil set (C117, B240)
func _Set(
	path *common.Trail,
	given *starlark.Set,
) (*structpb.Value, error) {
	if given == nil {
		return nil, _Refused(nil)
	}

	members := make(
		starlark.Tuple,
		0,
		given.Len(),
	)

	var (
		iterator = given.Iterate()
		member   starlark.Value
	)

	defer iterator.Done()

	for iterator.Next(&member) {
		members = append(
			members,
			member,
		)
	}

	return _List(
		path,
		given,
		members.Len(),
		members.Index,
	)
}

// _Number is an integer that fits a protobuf number.
//
// A protobuf number is a float64, which holds integers exactly only up to
// EXACT. Past that the integer would cross as a different number.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Number(held starlark.Int) (*structpb.Value, error) {
	exact, ok := held.Int64()
	if !ok || exact > EXACT || exact < -EXACT {
		return nil, fmt.Errorf(
			"%s does not fit a wire number: cannot be put on the wire: %w",
			unpack.ShortText(held.String()),
			unpack.ErrConvert,
		)
	}

	return structpb.NewNumberValue(float64(exact)), nil
}

// _List is a sequence as a protobuf list.
//
// Takes the length and the accessor rather than the value, because a list and
// a tuple answer the same two questions and do not share an interface that
// says so.
//
// container is the list, or nil for a tuple, which cannot hold itself.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host (C73, Q257)
func _List(
	path *common.Trail,
	container any,
	length int,
	at func(int) starlark.Value,
) (*structpb.Value, error) {
	err := trail.Enter(
		path,
		container,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	held := make(
		[]*structpb.Value,
		0,
		length,
	)

	for index := range length {
		part, err := Value(
			path,
			at(index),
		)
		if err != nil {
			return nil, err
		}

		held = append(
			held,
			part,
		)
	}

	return structpb.NewListValue(&structpb.ListValue{
		Values: held,
	}), nil
}

// _Dict is a dictionary as a protobuf struct.
//
// A protobuf struct keys by string, so a dictionary keyed by anything else
// cannot cross. That is a narrower rule than IsData, which allows a number as
// a key, and it is the one place the wire is stricter than the store.
//
// The pairs are walked through Entries rather than Items, which builds a slice
// of every pair first, only to be read once and dropped.
//
// Revisions:
//   - 2026-09-25 00:20: initial creation
//   - 2026-10-03 17:08: walks the pairs without first building a slice of them
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-07 19:54: moved to package host from remote (C91w8c, Q228)
//   - 2026-11-08 10:46: moved to package wire from host (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 11:54: refuses a nil dict (C117, B240)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func _Dict(
	path *common.Trail,
	given *starlark.Dict,
) (*structpb.Value, error) {
	if given == nil {
		return nil, _Refused(nil)
	}

	err := trail.Enter(
		path,
		given,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	held := map[string]*structpb.Value{}

	for name, value := range given.Entries() {
		key, ok := starlark.AsString(name)
		if !ok {
			return nil, fmt.Errorf(
				"a key of %s: cannot be put on the wire: %w",
				name.Type(),
				unpack.ErrConvert,
			)
		}

		part, err := Value(
			path,
			value,
		)
		if err != nil {
			return nil, err
		}

		held[key] = part
	}

	return structpb.NewStructValue(&structpb.Struct{
		Fields: held,
	}), nil
}
