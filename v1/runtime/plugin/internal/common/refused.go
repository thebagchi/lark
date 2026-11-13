package common

import (
	"fmt"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Refused is the error for text a script supplied that a builtin cannot read:
// who names the builtin, text is cut as unpack.QuotePrefix cuts it, and the
// sentinel stays in the chain for errors.Is.
//
// The message is "who got "text": sentinel". Every caller writes the quote
// through this one function, so none can forget the bound on a long text.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from eight copies in the plugins (C59, A4c)
func Refused(
	who string,
	text string,
	sentinel error,
) error {
	return fmt.Errorf(
		"%s got %s: %w",
		who,
		unpack.QuotePrefix(text),
		sentinel,
	)
}
