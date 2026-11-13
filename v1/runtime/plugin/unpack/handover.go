package unpack

import (
	"unsafe"
)

// Handover is a buffer that the caller built and owns, as a string over the
// same bytes. It is the one place where the runtime turns an output buffer
// into the text of a Starlark value without a second copy.
//
// The rule that makes it safe: after the call nothing writes the buffer, and
// nothing else keeps a slice of it. This is the one condition unsafe.String
// asks: "the bytes passed to String must not be modified as long as the
// returned string value exists". A caller makes the buffer for the call, fills
// it, and passes it here as its last use. The slice of a buffer that a caller
// keeps, a pool, a field or a script's value is not a buffer to pass.
//
// strings.Builder.String does the same, over its own buffer; bytes.Buffer.String
// copies, as a string(buf) conversion does. The copy was a second allocation
// as long as the output, on every call.
//
// The text goes into starlark.String, or starlark.Bytes, whose underlying type
// is string; both are immutable by the same rule.
//
// The size of the buffer is the caller's. This function allocates nothing and
// reads nothing of the input, so it adds no bound of its own.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37, plan D1)
func Handover(buffer []byte) string {
	return unsafe.String(
		unsafe.SliceData(buffer),
		len(buffer),
	)
}
