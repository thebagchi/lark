// Package proto holds Set, which puts one field into a Starlark dict and names
// the key when the dict refuses it, and the error for a value that cannot cross
// the data boundary. The conversions that call Set stay in package unpack.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package proto

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

var (
	// ErrConvert is returned for a value that cannot cross a data
	// boundary, and the wrap names which: FromProto for a protobuf value it
	// cannot convert (a kind this version does not know, or a dict that
	// refuses a key); state.set and state.update for a function or a handle,
	// which a store cannot usefully hold; event post for a value an event
	// cannot carry; remote for an argument that cannot cross a socket; and
	// runtime.Export for a value the wire cannot name.
	//
	// A store exists so that threads pass data to each other. A function is
	// code, and a frozen one read back by another thread is the same object the
	// script already had; a handle names a thread, and a thread means nothing
	// to whoever did not start it. Storing either is a mistake that reads as if
	// it worked.
	//
	// Revisions:
	//   - 2026-11-02 12:01: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/unpack (C120b, Q315)
	//   - 2026-11-10 17:15: renamed from ErrNotProtoValue (C121, Q349)
	ErrConvert = errors.New("value cannot be converted")
)

// Set puts one field in a dict and names the key when the dict refuses it.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/unpack and renamed _Set to Set, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func Set(
	into *starlark.Dict,
	key string,
	value starlark.Value,
) error {
	err := into.SetKey(
		starlark.String(key),
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: %w: %w",
			key,
			ErrConvert,
			err,
		)
	}

	return nil
}
