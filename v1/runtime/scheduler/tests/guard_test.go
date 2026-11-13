package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// TestGuard_NilThread proves the three calls that take a thread and report
// nothing back do not panic on a nil thread, and that Fail hands its cause back
// unchanged, as it does for a thread that carries no run.
//
// Cases come from Go-level breaker run 3, findings scheduler-F2, F3 and F4
// (bugs B057, B058, B059).
//
// Revisions:
//   - 2026-11-03 21:57: initial creation (C52)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_NilThread(t *testing.T) {
	var (
		cause = errors.New("cause")
		tests = []struct {
			name string
			call func() error
			want error
		}{
			{
				"Open does nothing",
				func() error {
					scheduler.Open(
						nil,
						new(common.Line),
					)

					return nil
				},
				nil,
			},
			{
				"Close does nothing",
				func() error {
					scheduler.Close(
						nil,
						"name",
						cause,
					)

					return nil
				},
				nil,
			},
			{
				"Fail returns the cause",
				func() error {
					return scheduler.Fail(
						nil,
						cause,
					)
				},
				cause,
			},
			{
				"Fail with a nil cause returns nil",
				func() error {
					return scheduler.Fail(
						(*starlark.Thread)(nil),
						nil,
					)
				},
				nil,
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := test.call()
				if !errors.Is(
					got,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}
