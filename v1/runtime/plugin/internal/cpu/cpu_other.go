//go:build !amd64 || purego

package cpu

// Detect is the portable level: this build has no kernels.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-03 22:30: moved from codec/hex, to be shared with base64 (C51)
func Detect() Level {
	return LEVEL_SWAR
}
