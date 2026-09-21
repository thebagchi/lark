package codec

import (
	"fmt"
	"hash/crc32"

	"go.starlark.net/starlark"
)

// _CRC32 is the IEEE checksum of data, unsigned, optionally continuing one
// already started.
//
// The second argument is what makes a checksum of something too big to hold
// possible: a script reads a chunk, keeps the number, and hands it back for
// the next. binascii's crc32 takes it for the same reason.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _CRC32(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given starlark.Value
		seed  = starlark.MakeInt(NO_SEED)
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given, &seed)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	data, err := _Bytes(fn.Name(), given)
	if err != nil {
		return nil, err
	}

	held, ok := seed.Uint64()
	if !ok {
		return nil, fmt.Errorf("%s got %s to continue: %w", fn.Name(), seed, ErrRange)
	}

	return starlark.MakeUint64(uint64(crc32.Update(uint32(held), crc32.IEEETable, data))), nil
}
