//go:build !amd64 || purego

package codec

// _EncodeKernel handles no bytes: this build has no kernels.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 and takes *Coding, renamed from
//     *_Coding (C120b, Q315, Q314)
func _EncodeKernel(
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	return 0
}

// _DecodeKernel handles no characters: this build has no kernels.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/base64 and takes *Coding, renamed from
//     *_Coding (C120b, Q315, Q314)
func _DecodeKernel(
	coding *Coding,
	dst []byte,
	src []byte,
) int {
	return 0
}
