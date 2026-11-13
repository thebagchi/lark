package tests

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/spelling"
	"github.com/thebagchi/lark/v1/runtime/spelling/internal/spec"
)

// TestChild_SpellsTheIdOfAChild pins the text of a thread id: the spine's
// children carry no prefix of the spine, every other child names its parent,
// and an ordinal is written in decimal whatever its width.
//
// Revisions:
//   - 2026-10-29 01:37: initial creation
//   - 2026-11-05 19:07: the extreme ordinals are math.MaxInt and math.MinInt, for 32 bits
//     (C58, B119)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestChild_SpellsTheIdOfAChild(t *testing.T) {
	cases := []struct {
		name    string
		parent  string
		ordinal int
		want    string
	}{
		{
			name:    "spine zero",
			parent:  common.SPINE,
			ordinal: 0,
			want:    "thread_0",
		},
		{
			name:    "spine one",
			parent:  common.SPINE,
			ordinal: 1,
			want:    "thread_1",
		},
		{
			name:    "spine nine",
			parent:  common.SPINE,
			ordinal: 9,
			want:    "thread_9",
		},
		{
			name:    "spine ten",
			parent:  common.SPINE,
			ordinal: 10,
			want:    "thread_10",
		},
		{
			name:    "spine large",
			parent:  common.SPINE,
			ordinal: 1234567,
			want:    "thread_1234567",
		},
		{
			name:    "child zero",
			parent:  "thread_1",
			ordinal: 0,
			want:    "thread_1_0",
		},
		{
			name:    "child nine",
			parent:  "thread_1",
			ordinal: 9,
			want:    "thread_1_9",
		},
		{
			name:    "child ten",
			parent:  "thread_1",
			ordinal: 10,
			want:    "thread_1_10",
		},
		{
			name:    "child large",
			parent:  "thread_12_3",
			ordinal: 4096,
			want:    "thread_12_3_4096",
		},
		{
			name:    "deep parent",
			parent:  "thread_123_45_67_89_101_112",
			ordinal: 256,
			want:    "thread_123_45_67_89_101_112_256",
		},
		{
			name:    "largest int",
			parent:  "thread_1",
			ordinal: math.MaxInt,
			want:    "thread_1_" + strconv.Itoa(math.MaxInt),
		},
		{
			name:    "smallest int",
			parent:  common.SPINE,
			ordinal: math.MinInt,
			want:    "thread_" + strconv.Itoa(math.MinInt),
		},
		{
			name:    "empty parent",
			parent:  "",
			ordinal: 3,
			want:    "_3",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := spelling.Child(
					tc.parent,
					tc.ordinal,
				)
				if got != tc.want {
					t.Fatalf(
						"Child(%q, %d) = %q, want %q",
						tc.parent,
						tc.ordinal,
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestNearest_SuggestsOneCloseName pins the suggestion rule: the one nearest
// candidate within an edit distance of under half the word's length, ignoring
// case and underscores, the first in sorted order on a tie, and none when
// nothing is close or the word is too long to compare.
//
// Revisions:
//   - 2026-11-05 08:07: initial creation (C70)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNearest_SuggestsOneCloseName(t *testing.T) {
	var (
		names = []string{"print", "join", "spawn", "sleep", "script", "args"}
		long  = strings.Repeat(
			"a",
			spec.MAX_WORD+1,
		)
	)

	cases := []struct {
		name  string
		word  string
		names []string
		want  string
	}{
		{
			"one letter missing",
			"prnt",
			names,
			"print",
		},
		{
			"one letter changed",
			"spwan",
			names,
			"spawn",
		},
		{
			"case and underscores ignored",
			"S_PAWN",
			names,
			"spawn",
		},
		{
			"flag typo",
			"scrpt",
			names,
			"script",
		},
		{
			"nothing close",
			"zebra",
			names,
			"",
		},
		{
			"short word has no room",
			"j",
			names,
			"",
		},
		{
			"empty word",
			"",
			names,
			"",
		},
		{
			"no candidates",
			"prnt",
			nil,
			"",
		},
		{
			"tie goes to the first sorted",
			"abc",
			[]string{"abe", "abd"},
			"abd",
		},
		{
			"word beyond the bound",
			long,
			[]string{long + "b"},
			"",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := spelling.Nearest(
					tc.word,
					tc.names,
				)
				if got != tc.want {
					t.Errorf(
						"Nearest(%q) = %q, want %q",
						tc.word,
						got,
						tc.want,
					)
				}
			},
		)
	}
}
