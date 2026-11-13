package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

// TestCalled_NamesABuiltinOrANilOne proves a builtin gives its name and a nil
// builtin gives "builtin" in place of panicking (C100b).
//
// Revisions:
//   - 2026-11-06 00:22: initial creation (C100b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCalled_NamesABuiltinOrANilOne(t *testing.T) {
	tests := []struct {
		name string
		fn   *starlark.Builtin
		want string
	}{
		{
			name: "named",
			fn: starlark.NewBuiltin(
				"wait",
				nil,
			),
			want: "wait",
		},
		{
			name: "nil",
			fn:   nil,
			want: "builtin",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				if got := common.Called(test.fn); got != test.want {
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
