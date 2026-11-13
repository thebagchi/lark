package command

import (
	"slices"
	"strings"
)

// HelpOf is the part of text, the help of the whole command, that belongs to
// verb: the usage line of the verb, the paragraph on FILE for a verb that takes
// one, the flags of the verb, and the exit codes. It is "" for a word that is
// no verb.
//
// The text is read by its paragraphs, which are set apart by a blank line: the
// one that opens "Usage:", the one that opens "FILE:", the one that opens with
// the verb and a colon, and the one that opens "Exit codes:". The help text
// stays the one source of the words (C68).
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305, Q310)
func HelpOf(
	text string,
	verb string,
) string {
	if !slices.Contains(
		_Verbs(),
		verb,
	) {
		return ""
	}

	var (
		paragraphs = strings.Split(
			text,
			"\n\n",
		)
		kept = []string{"Usage:\n" + _Usage(
			_Paragraph(
				paragraphs,
				"Usage:\n",
			),
			verb,
		)}
	)

	if slices.Contains(
		[]string{RUN, COMPILE, CONVERT},
		verb,
	) {
		kept = append(
			kept,
			_Paragraph(
				paragraphs,
				"FILE:",
			),
		)
	}

	for _, opening := range []string{
		verb + ":\n",
		"Exit codes:",
	} {
		found := _Paragraph(
			paragraphs,
			opening,
		)
		if found != "" {
			kept = append(
				kept,
				found,
			)
		}
	}

	return strings.Join(
		kept,
		"\n\n",
	) + "\n"
}

// _Usage is the line of usage that belongs to verb: the one that starts with
// two spaces, "lark", the verb and a space.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
func _Usage(
	usage string,
	verb string,
) string {
	for _, line := range strings.Split(
		usage,
		"\n",
	) {
		if strings.HasPrefix(
			line,
			"  lark "+verb+" ",
		) {
			return line
		}
	}

	return ""
}

// _Paragraph is the first of paragraphs that starts with opening, with the
// newline that ends it taken off, or "" when none does.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C119, Q305)
func _Paragraph(
	paragraphs []string,
	opening string,
) string {
	for _, paragraph := range paragraphs {
		if strings.HasPrefix(
			paragraph,
			opening,
		) {
			return strings.TrimSuffix(
				paragraph,
				"\n",
			)
		}
	}

	return ""
}
