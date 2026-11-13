// Package spelling is the vocabulary a script and a graph both name.
//
// The entry point, the thread ids, the builtins that name threads rather than
// values. Four packages used to declare these separately - graph said so in a
// comment, that generating a script should not depend on compiling one - and a
// test compared them pair by pair to catch the drift. The reason was right and
// the remedy was a copy: this package is the reason without the copy, because it
// depends only on common, the data package, and on no package of the
// interpreter or the compiler, so importing it costs neither side a behaviour.
// The names are declared in common (Q244) and the rest of the constants in
// spelling/internal/spec.
//
// Nothing here has behaviour beyond Child and Nearest. It is names, one rule about how an
// id is built from them, and the two mistakes both sides refuse, declared once
// so that errors.Is matches whichever side found one.
//
// Revisions:
//   - 2026-09-27 00:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: depends on common, not on nothing: the names moved there (Q244, C91w12d)
package spelling

import (
	"errors"
	"slices"
	"strconv"
	"unicode"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/spelling/internal/spec"
)

var (
	// ErrArgs is returned for a call passing a function arguments it does not
	// take, whether a compile finds it in a script or a check finds it in a
	// flow.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-13 23:55: renamed from ErrArity (C136, Q475)
	ErrArgs = errors.New("a call passes arguments the function does not take")

	// ErrMain is returned for a script that defines no entry point, whether
	// it was to be run or read into a flow.
	//
	// The text ends with the fix, in the one style of a hint: parentheses
	// after the failing text. A person who has not met the term "entry point"
	// reads def main() and the word module.
	//
	// Revisions:
	//   - 2026-11-05 08:07: the text names def main() and the module (C70)
	//   - 2026-11-10 17:15: renamed from ErrNoMain (C121, Q348)
	ErrMain = errors.New(
		"no entry point (define def main() with no required " +
			"parameters; a file without it is a module, for load)",
	)

	// ErrCycle is returned for a script whose modules load each other in a
	// ring, whether it was being compiled or read into a flow.
	//
	// Revisions:
	//   - 2026-09-19 23:36: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ErrCycle = errors.New("cycle in the load graph")

	// ErrDepth is returned for a chain of loads longer than MAX_DEPTH.
	// A chain that long overflows the stack before it returns an error.
	//
	// Revisions:
	//   - 2026-10-28 18:20: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ErrDepth = errors.New("load chain too deep")
)

// Child is the id of the ordinal-th thread started by parent.
//
// An id names its parent, so structure can be read off it: thread_1_2 was
// started by thread_1. The spine is the exception, because thread_0_1 would
// carry a 0 that says nothing - every id descends from the spine.
//
// The ordinal is counted per parent rather than per run. One counter shared by
// every thread would number in the order spawns happen, which is a fact about
// time; an id naming its parent is a fact about structure, and the two disagree
// whenever a spawned function spawns before its siblings start.
//
// Revisions:
//   - 2026-09-27 00:10: initial creation, from the copies in scheduler and graph
//   - 2026-10-29 01:37: writes the ordinal into a buffer on the stack and joins
//     it to the parent in one allocation, rather than through fmt, which every
//     spawn paid for and which boxed a parent in a second one
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func Child(
	parent string,
	ordinal int,
) string {
	var room [spec.DIGITS]byte

	digits := strconv.AppendInt(
		room[:0],
		int64(ordinal),
		spec.DECIMAL,
	)

	if parent == common.SPINE {
		return common.THREAD + string(digits)
	}

	return parent + "_" + string(digits)
}

// Nearest is the one candidate nearest to word, or "" when none is close.
// A caller writes the suggestion as the hint style of Q158: `<text> (did you
// mean X?)`, in lower case.
//
// Close means an edit distance (Levenshtein, over runes) under half the length
// of word, rounded up: "prnt" is one edit from "print", and "zebra" is none
// from any of the names of a script. Case and underscores do not count, so
// "S_PAWN" is "spawn". On a tie the first candidate in sorted order wins, so
// the answer does not depend on the order of the slice.
//
// It gives at most one suggestion, as go.starlark.net does for a name in its
// resolver ("did you mean X?"); that rule and its limit are the ones copied
// here, because the starlark package keeps its own copy internal. A word of
// more than MAX_WORD bytes gets no suggestion.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation, for the hints of change C70
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func Nearest(
	word string,
	candidates []string,
) string {
	if word == "" || len(word) > spec.MAX_WORD {
		return ""
	}

	var (
		sorted = slices.Sorted(slices.Values(candidates))
		folded = _Folded(word)
		limit  = (len(folded) + 1) / 2
		best   string
	)

	for _, candidate := range sorted {
		distance := _Distance(
			folded,
			_Folded(candidate),
			limit,
		)
		if distance < limit {
			limit = distance
			best = candidate
		}
	}

	return best
}

// _Folded is text in lower case with its underscores taken out.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation, for Nearest (C70)
func _Folded(text string) []rune {
	folded := make(
		[]rune,
		0,
		len(text),
	)

	for _, r := range text {
		if r == '_' {
			continue
		}

		folded = append(
			folded,
			unicode.ToLower(r),
		)
	}

	return folded
}

// _Distance is the Levenshtein distance between a and b, or a number of at
// least limit when it is that large: a row that has passed limit ends the work.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation, for Nearest (C70)
func _Distance(
	a []rune,
	b []rune,
	limit int,
) int {
	if len(a) < len(b) {
		a, b = b, a
	}

	if len(a)-len(b) >= limit {
		return limit
	}

	row := make(
		[]int,
		len(b)+1,
	)
	for j := range row {
		row[j] = j
	}

	for i := 1; i <= len(a); i++ {
		diagonal := row[0]
		row[0] = i
		least := row[0]

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			next := min(
				row[j]+1,
				row[j-1]+1,
				diagonal+cost,
			)
			diagonal = row[j]
			row[j] = next
			least = min(
				least,
				next,
			)
		}

		if least >= limit {
			return limit
		}
	}

	return row[len(b)]
}
