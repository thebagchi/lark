package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
)

// TestHash_BufferAndReaderCannotBeKeys proves B052 is not a bug: the Hash of a
// Buffer and of a Reader refuses, so a dict cannot take either as a key and
// mutating one cannot corrupt a dict. go.starlark.net allows Hash to fail "if
// the value's type is not hashable" (starlark/value.go, Value.Hash).
//
// Revisions:
//   - 2026-11-05 17:05: initial creation (B052, C96)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHash_BufferAndReaderCannotBeKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  starlark.Value
	}{
		{
			"buffer",
			new(buffer.Buffer),
		},
		{
			"reader",
			new(buffer.Reader),
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if _, err := tc.key.Hash(); err == nil {
					t.Fatal("Hash succeeded, want a refusal")
				}

				var (
					dict = starlark.NewDict(1)
					err  = dict.SetKey(
						tc.key,
						starlark.None,
					)
				)
				if err == nil {
					t.Fatal("dict took the value as a key, want a refusal")
				}
			},
		)
	}
}
