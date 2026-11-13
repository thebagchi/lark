package buffer

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Values returns the buf module. Package buf hands it to the registry.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: a function here, which _Buf.Values calls (C91w8a)
//   - 2026-11-10 01:00: now uses NewBuffer, were _NewBuffer (C120a, Q314)
func Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				NEW: starlark.NewBuiltin(
					NAME+"."+NEW,
					NewBuffer,
				),
				READER: starlark.NewBuiltin(
					NAME+"."+READER,
					_NewReader,
				),
			},
		},
	}
}

// NewBuffer is an empty buffer.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved from package buf (C91w8a)
//   - 2026-11-10 00:25: renamed from _NewBuffer, as a test in tests/ needs it (C120a, Q314)
func NewBuffer(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
	)
	if err != nil {
		return nil, err
	}

	return &Buffer{}, nil
}

// _NewReader is a reader over data, bytes or the UTF-8 bytes of a str, from
// its first byte.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved from package buf (C91w8a)
func _NewReader(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return &Reader{
		data: data,
	}, nil
}
