//go:build !amd64 || purego

package codec

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// _EncodeKernel reports false: this build has no kernels.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _EncodeKernel(
	level cpu.Level,
	dst []byte,
	src []byte,
) bool {
	return false
}

// _DecodeKernel reports false: this build has no kernels.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:50: the level comes from internal/cpu, shared with base64 (C51)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/codec/hex (C120b, Q315)
func _DecodeKernel(
	level cpu.Level,
	dst []byte,
	src []byte,
) (bool, bool) {
	return false, false
}
