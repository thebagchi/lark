package spelling_test

import (
	"math"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// TestChild_SpellsTheIdOfAChild pins the text of a thread id: the spine's
// children carry no prefix of the spine, every other child names its parent,
// and an ordinal is written in decimal whatever its width.
//
// Revisions:
//   - 2026-10-29 01:37: initial creation
func TestChild_SpellsTheIdOfAChild(t *testing.T) {
	cases := []struct {
		name    string
		parent  string
		ordinal int
		want    string
	}{
		{
			name:    "spine zero",
			parent:  spelling.SPINE,
			ordinal: 0,
			want:    "thread_0",
		},
		{
			name:    "spine one",
			parent:  spelling.SPINE,
			ordinal: 1,
			want:    "thread_1",
		},
		{
			name:    "spine nine",
			parent:  spelling.SPINE,
			ordinal: 9,
			want:    "thread_9",
		},
		{
			name:    "spine ten",
			parent:  spelling.SPINE,
			ordinal: 10,
			want:    "thread_10",
		},
		{
			name:    "spine large",
			parent:  spelling.SPINE,
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
			ordinal: math.MaxInt64,
			want:    "thread_1_9223372036854775807",
		},
		{
			name:    "smallest int",
			parent:  spelling.SPINE,
			ordinal: math.MinInt64,
			want:    "thread_-9223372036854775808",
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
