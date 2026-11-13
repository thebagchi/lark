package unpack

import (
	"strings"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/spec"
)

// CheckPadding is the text of a base-N encoding with its padding taken off, and
// whether the text is one the encoding may be written as: no CR or LF, and
// either no padding or exactly the padding that fills the last group.
//
// group is how many characters one group holds: 4 for base64, 8 for base32.
//
// Why this is checked here and not left to the standard library:
//
//   - RFC 4648 section 3.3: "Implementations MUST reject the encoded data if it
//     contains characters outside the base alphabet ... unless the specification
//     referring to this document explicitly states otherwise." A CR or LF is such
//     a character ("any adjacent carriage return/line feed (CRLF) characters
//     constitute "non-alphabet characters""). Go skips them even in Strict mode.
//   - The same section lets a referring specification ignore excess "=" ("the
//     excess pad characters MAY also be ignored"). lark states no such rule, so
//     "Zg===" is refused. Two spellings of one value is the risk of RFC 4648
//     section 12.
//   - Text with no padding is kept: RFC 4648 section 3.2 lets a specification omit it, and a
//     JSON Web Token (RFC 7515 section 2) does.
//
// A count of "=" that does not fill the group returns false, and so does any
// "=" after a whole group. A group that is not above zero cannot be filled, so
// padding after it is refused.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-04 23:50: refuses padding after a group of zero or less, which
//     divided by zero (C86 fix round 3)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func CheckPadding(
	text string,
	group int,
) (string, bool) {
	if strings.ContainsAny(
		text,
		spec.LINE_BREAKS,
	) {
		return "", false
	}

	var (
		bare = strings.TrimRight(
			text,
			spec.PADDING,
		)
		padding = len(text) - len(bare)
	)

	if padding == 0 {
		return bare, true
	}

	if group <= 0 {
		return bare, false
	}

	// A group is complete at remainder 0, so padding there is excess.
	remainder := len(bare) % group

	return bare, remainder != 0 && padding == group-remainder
}
