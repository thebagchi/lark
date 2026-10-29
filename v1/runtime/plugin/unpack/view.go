package unpack

import (
	"unsafe"

	"go.starlark.net/starlark"
)

// DataView reads a builtin's one argument as bytes that share the memory of
// the script's value. It is Data with the contract of View, for a builtin that
// takes nothing but its data.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
//   - 2026-10-29 23:58: checks the call with Positional, so no local goes to
//     the heap
func DataView(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]byte, error) {
	err := Positional(
		fn,
		args,
		kwargs,
		1,
	)
	if err != nil {
		return nil, err
	}

	return View(
		fn.Name(),
		args[0],
	)
}

// View is one value as bytes that share its memory, with no copy. It is a
// second contract beside Bytes, and narrower: the caller only reads the bytes,
// and keeps nothing of them once it returns.
//
// A Starlark string or bytes is immutable, and a script still holds the value.
// A write through the view changes that value under the script, and a slice
// kept past the call sees a value the caller no longer owns. Use Bytes when
// either can happen.
//
// The copy that Bytes makes is the whole input, on every call. An encoder or a
// digest reads its input once, so the copy was a second pass and an allocation
// as large as the input. A refusal is the refusal of Bytes, so the two
// contracts refuse the same values with the same text.
//
// Revisions:
//   - 2026-10-29 01:08: initial creation
func View(
	who string,
	given starlark.Value,
) ([]byte, error) {
	var text string

	switch held := given.(type) {
	case starlark.Bytes:
		text = string(held)

	case starlark.String:
		text = string(held)

	default:
		return Bytes(
			who,
			given,
		)
	}

	return unsafe.Slice(
		unsafe.StringData(text),
		len(text),
	), nil
}
