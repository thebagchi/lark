package merge

import (
	"bytes"
	"sort"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// _Stripped is the edits of the piece and one edit for each comment of its
// text, in order of place.
//
// A comment is a # outside every string. The strings are the Literal nodes of
// the parsed file: syntax.Literal has the position of its token and the raw
// text of it, quotes included, so a # between those two is part of a string.
// The parser keeps no comment that a tree can lose; RetainComments hangs a
// comment on a node, and drops a suffix comment when too few nodes end before
// it (tested: "def f():  # c" with a second comment later in the file). A
// string is the only token that may hold a #, so the text between the strings
// is scanned. An odd text (see graph.Odd) keeps its comments. The blanks before a
// comment go with it, and a comment inside what an edit already replaced is
// left to that edit.
//
// The string ranges are found when the file is merged (see _Module._Spans),
// from the tree the merge holds, so rendering the text parses nothing.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-05 18:00: reads the ranges the merge found, in place of parsing
//     the text again, and cannot fail (Q157, item 4)
func (p *_Piece) _Stripped() []*_Edit {
	if !p.strip {
		return p.edits
	}

	edits := make(
		[]*_Edit,
		0,
		len(p.edits),
	)
	edits = append(
		edits,
		p.edits...,
	)
	edits = append(
		edits,
		p._Comments(p.spans)...,
	)

	sort.Slice(
		edits,
		func(
			i,
			j int,
		) bool {
			return edits[i].start < edits[j].start
		},
	)

	return edits
}

// _Comments is one edit for each # of the text that stands outside the string
// ranges spans, which are in order of place, from it to the end of its line.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from _Stripped (C44b)
func (p *_Piece) _Comments(spans [][2]int) []*_Edit {
	var (
		edits []*_Edit
		next  = 0
		at    = 0
	)

	for at < len(p.text) {
		if next < len(spans) && at >= spans[next][0] {
			at = max(
				at,
				spans[next][1],
			)
			next++

			continue
		}

		if p.text[at] != '#' {
			at++

			continue
		}

		end := bytes.IndexByte(
			p.text[at:],
			'\n',
		)
		if end < 0 {
			end = len(p.text)
		} else {
			end += at
		}

		if !p._Covered(
			at,
			end,
		) {
			edits = append(
				edits,
				&_Edit{
					start: p._Blanks(at),
					end:   end,
				},
			)
		}

		at = end
	}

	return edits
}

// _Spans is the byte range of every string of the module, in order of place,
// when the module has a comment to strip, and false when it has none or its
// text is odd or a literal is not where the tree says it is.
//
// It reads the tree the merge has resolved, so no text is parsed twice.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation, from _Strings (Q157, item 4)
//   - 2026-11-05 23:26: finds the places of the literals with _Module._At, not a lines value
//     of its own (C60, Q188)
func (m *_Module) _Spans() ([][2]int, bool) {
	if bytes.IndexByte(
		m.text,
		'#',
	) < 0 || graph.Odd(m.text) {
		return nil, false
	}

	var (
		ranges [][2]int
		ok     = true
	)

	syntax.Walk(
		m.tree,
		func(node syntax.Node) bool {
			literal, found := node.(*syntax.Literal)
			if !found ||
				(literal.Token != syntax.STRING && literal.Token != syntax.BYTES) {
				return true
			}

			var (
				start = m._At(literal.TokenPos)
				end   = start + len(literal.Raw)
			)

			if end > len(m.text) || string(m.text[start:end]) != literal.Raw {
				ok = false

				return true
			}

			ranges = append(
				ranges,
				[2]int{start, end},
			)

			return true
		},
	)

	sort.Slice(
		ranges,
		func(
			i,
			j int,
		) bool {
			return ranges[i][0] < ranges[j][0]
		},
	)

	return ranges, ok
}

// _Covered reports whether an edit of the piece replaces any of the bytes from
// start to end.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
func (p *_Piece) _Covered(
	start int,
	end int,
) bool {
	for _, edit := range p.edits {
		if start < edit.end && end > edit.start {
			return true
		}
	}

	return false
}

// _Blanks is the offset the blanks before start begin at, or start itself when
// no space or tab stands before it, or an edit ends at the first of them.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
func (p *_Piece) _Blanks(start int) int {
	at := start

	for at > 0 && (p.text[at-1] == ' ' || p.text[at-1] == '\t') {
		at--
	}

	for _, edit := range p.edits {
		if edit.end > at && edit.end <= start {
			return start
		}
	}

	return at
}
