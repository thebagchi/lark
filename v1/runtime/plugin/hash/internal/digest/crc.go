// Package digest holds the checksum side of the hash plugin: crc32, hmac and the
// table of hash makers they use. The plugin type, its registration and the plain
// digests stay in package hash.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package digest

import (
	"fmt"
	"hash/crc32"
	"math"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// CRC32 is the IEEE checksum of data, unsigned, optionally continuing one
// already started.
//
// The second argument is what makes a checksum of something too big to hold
// possible: a script reads a chunk, keeps the number, and hands it back for
// the next. binascii's crc32 takes it for the same reason.
//
// A seed is a checksum, so it is thirty-two bits and anything wider is
// refused. Cutting it to width instead made 2**32 the same seed as 0, which is
// a wrong answer given confidently.
//
// The data is a view of the script's value, not a copy: crc32.Update only
// reads it, which is the contract of unpack.View.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's crc32
//   - 2026-09-24 16:08: refuses a seed too wide to be a checksum, rather than
//     truncating it
//   - 2026-10-08 17:47: a member of the hash module, refusing with unpack's ErrRange
//   - 2026-10-29 01:08: reads a view of the data rather than a copy
//   - 2026-11-02 08:41: returns the refusal of the arguments as it is, so the
//     builtin name appears once
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash and renamed _CRC32 to CRC32, for the
//     tests in tests/ (C120b, Q315, Q314)
func CRC32(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given starlark.Value
		seed  = starlark.MakeInt(spec.NO_SEED)
	)

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
		&seed,
	)
	if err != nil {
		return nil, err
	}

	data, err := unpack.View(
		shared.Called(fn),
		given,
	)
	if err != nil {
		return nil, err
	}

	held, ok := seed.Uint64()
	if !ok || held > math.MaxUint32 {
		return nil, fmt.Errorf(
			"%s got %s to continue: %w",
			shared.Called(fn),
			unpack.ShortText(seed.String()),
			unpack.ErrRange,
		)
	}

	return starlark.MakeUint64(uint64(crc32.Update(
		uint32(held),
		crc32.IEEETable,
		data,
	))), nil
}
