// This file tests LocatedError against a nil receiver, a nil wrapped error and a
// racing store, through its fields and its methods Error, Unwrap, Say and Says
// (the breaker guards of C112, Q65, Q206). No error that Locate returns is nil
// or races. The tests of Locate itself are in locate_test.go.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: the tests that need no unexported name moved to the external package; this
//     comment names what stays (C65c)
//   - 2026-11-10 00:25: moved to tests/ from locate_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"errors"
	"sync"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestGuard_C112_LocatedErrorNil proves Error gives text and does not panic on
// a nil receiver or a nil wrapped error.
//
// Cases from the Go-level breaker, second pass, findings scheduler-F11 and F13
// (Q65, Q206). The error text keeps the place and drops the words it has none
// of.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q65, Q206)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C112_LocatedErrorNil(t *testing.T) {
	tests := []struct {
		name  string
		build func() *run.LocatedError
		want  string
	}{
		{
			name: "nil receiver",
			build: func() *run.LocatedError {
				return nil
			},
			want: "",
		},
		{
			name: "nil err with words",
			build: func() *run.LocatedError {
				located := &run.LocatedError{
					Where: "x.star:2",
				}
				located.Say("boom")

				return located
			},
			want: "x.star:2: boom",
		},
		{
			name: "nil err with no words",
			build: func() *run.LocatedError {
				return &run.LocatedError{
					Where: "x.star:2",
				}
			},
			want: "x.star:2",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got := tt.build().Error()
				if got != tt.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

// TestGuard_C112_LocatedErrorNilReceiver proves the other methods of a nil
// LocatedError give nothing and do not panic.
//
// Case from finding B230: Unwrap dereferenced the receiver (Q65).
//
// Revisions:
//   - 2026-11-08 20:57: initial creation (C112 fix round 1, B230)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C112_LocatedErrorNilReceiver(t *testing.T) {
	var located *run.LocatedError

	if located.Unwrap() != nil {
		t.Fatal("a nil located error unwraps to an error")
	}

	if located.Says() != "" {
		t.Fatal("a nil located error says something")
	}

	located.Say("ignored")
}

// TestGuard_C112_LocatedErrorSaysAgainstError proves a store to says does not
// race Error. Run with -race.
//
// Cases from the Go-level breaker, second pass, findings scheduler-F12 and F14
// (Q207). The breaker assigned the field from a second goroutine; says is
// guarded, so the write goes through Say.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C112_LocatedErrorSaysAgainstError(t *testing.T) {
	var (
		located = &run.LocatedError{
			Where: "w",
			Err:   errors.New("text_s"),
		}
		group sync.WaitGroup
	)

	group.Add(1)

	go func() {
		defer group.Done()

		for range 1000 {
			located.Say("s")
			located.Say("a much longer text than the error holds")
		}
	}()

	for range 1000 {
		_ = located.Error()
	}

	group.Wait()
}
