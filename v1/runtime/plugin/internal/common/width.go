package common

import (
	"fmt"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Width refuses a width below one with unpack.ErrRange, in the words every
// builtin that takes a width uses: "who got a width of N: out of range".
//
// It checks the sign only. A caller that makes a buffer of the width also
// asks memwatch.Room, and a caller that only reads does not.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from two copies (C59, A4d)
func Width(
	who string,
	width int,
) error {
	if width >= 1 {
		return nil
	}

	return fmt.Errorf(
		"%s got a width of %d: %w",
		who,
		width,
		unpack.ErrRange,
	)
}
