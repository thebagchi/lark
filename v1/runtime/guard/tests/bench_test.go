package tests

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/guard"
)

// BenchmarkWithRecover measures the ordinary path: fn returns, nothing panics.
//
// Revisions:
//   - 2026-11-02 23:44: initial creation, for C48
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkWithRecover(b *testing.B) {
	var (
		got int
		err error
	)

	body := func() (int, error) {
		return VALUE, nil
	}

	b.ReportAllocs()

	for b.Loop() {
		guard.WithRecover(
			&got,
			&err,
			body,
		)
	}
}
