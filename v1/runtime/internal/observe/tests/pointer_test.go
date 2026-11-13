package tests

// This file tests Pointer, the path builder, and the paths it makes (C65a).
//
// Revisions:
//   - 2026-11-08 22:49: header states why the file is internal (C65a)
//   - 2026-11-10 00:25: moved to tests/ from pointer_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// TestPointer_PathsAreAsTheStringBuilderMadeThem pins Pointer to the paths
// the first version made, captured by running these cases on the code before
// the stack buffer. The values stay fixed so the op stream stays as recorded.
// The cases with "/" and "~" show no escaping (RFC 6901 section 3 asks for ~1
// and ~0), which is harmless because no name is ever part of a path.
//
// Revisions:
//   - 2026-11-02 16:18: initial creation
//   - 2026-11-02 16:35: states in the comment why the values are pinned
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestPointer_PathsAreAsTheStringBuilderMadeThem(t *testing.T) {
	var long = strings.Repeat(
		"n",
		200,
	)

	var tests = []struct {
		name  string
		parts []any
		want  string
	}{
		{
			"none",
			nil,
			"",
		},
		{
			"one name",
			[]any{"status"},
			"/status",
		},
		{
			"name and index",
			[]any{"functions", 12, "status"},
			"/functions/12/status",
		},
		{
			"zero",
			[]any{"a", 0},
			"/a/0",
		},
		{
			"negative",
			[]any{"a", -7},
			"/a/-7",
		},
		{
			"big index",
			[]any{"a", 1234567890},
			"/a/1234567890",
		},
		{
			"empty name",
			[]any{""},
			"/",
		},
		{
			"slash in name",
			[]any{"a/b", 1},
			"/a/b/1",
		},
		{
			"tilde in name",
			[]any{"a~b", "~1"},
			"/a~b/~1",
		},
		{
			"unicode",
			[]any{"é", 3},
			"/é/3",
		},
		{
			"long past the stack buffer",
			[]any{long, 4, long},
			"/" + long + "/4/" + long,
		},
		{
			"other type is skipped",
			[]any{"a", 1.5, "b"},
			"/a//b",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := observe.Pointer(test.parts...)
				if got != test.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						test.want,
					)
				}
			},
		)
	}
}
