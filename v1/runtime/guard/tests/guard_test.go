package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/guard"
)

var (
	ErrBoom = errors.New("guard: boom")
)

// WHY is the text a contained panic carries, so a test can look for it.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	WHY = "a host's callback blew up"
)

const (
	VALUE     = 7
	TEXT      = "not an error"
	RECOVERED = "recovered"
)

// TestWithRecover_PassesAValueThrough proves the ordinary path is untouched.
//
// Revisions:
//   - 2026-09-19 21:19: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_PassesAValueThrough(t *testing.T) {
	var (
		got int
		err error
	)

	guard.WithRecover(
		&got,
		&err,
		func() (int, error) {
			return VALUE, nil
		},
	)

	if err != nil || got != VALUE {
		t.Fatalf(
			"got %d, %v, want %d, nil",
			got,
			err,
			VALUE,
		)
	}
}

// TestWithRecover_PassesAnErrorThrough proves a returned error is not touched.
//
// Revisions:
//   - 2026-09-19 21:19: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_PassesAnErrorThrough(t *testing.T) {
	var (
		got int
		err error
	)

	guard.WithRecover(
		&got,
		&err,
		func() (int, error) {
			return 0, ErrBoom
		},
	)

	if !errors.Is(
		err,
		ErrBoom,
	) {
		t.Fatalf(
			"got %v, want ErrBoom",
			err,
		)
	}
}

// TestWithRecover_KeepsAPanickedErrorReachable proves errors.Is still works
// through the recovery, which is why a panic carrying an error is not wrapped
// with %v.
//
// Revisions:
//   - 2026-09-19 21:20: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_KeepsAPanickedErrorReachable(t *testing.T) {
	var (
		got int
		err error
	)

	guard.WithRecover(
		&got,
		&err,
		func() (int, error) {
			panic(ErrBoom)
		},
	)

	if !errors.Is(
		err,
		ErrBoom,
	) {
		t.Fatalf(
			"got %v, want ErrBoom reachable through the recovery",
			err,
		)
	}

	if got != 0 {
		t.Fatalf(
			"result is %d, want the zero value after a panic",
			got,
		)
	}

	t.Logf(
		"panicked error survives errors.Is: %v",
		err,
	)
}

// TestWithRecover_WrapsAPanicThatIsNotAnError proves a panic of any other type
// still becomes an error rather than escaping.
//
// Revisions:
//   - 2026-09-19 21:21: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_WrapsAPanicThatIsNotAnError(t *testing.T) {
	var (
		got int
		err error
	)

	guard.WithRecover(
		&got,
		&err,
		func() (int, error) {
			panic(TEXT)
		},
	)

	if err == nil || !strings.Contains(
		err.Error(),
		TEXT,
	) {
		t.Fatalf(
			"got %v, want an error naming %q",
			err,
			TEXT,
		)
	}

	if !strings.Contains(
		err.Error(),
		RECOVERED,
	) {
		t.Fatalf(
			"error does not say it was recovered: %v",
			err,
		)
	}
}

// TestContained_HandsBackThePanic records what Contained is for: work done for
// its effect, whose failure still has to reach somebody.
//
// It returns the panic rather than absorbing it, which is the whole difference
// from swallowing one. A caller decides what the failure means; none of them
// may decide to ignore it in silence.
//
// Revisions:
//   - 2026-09-20 11:58: initial creation
//   - 2026-09-20 12:00: asks for the panic back, since Contain absorbed it
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestContained_HandsBackThePanic(t *testing.T) {
	blown := guard.Contained(func() {
		panic(WHY)
	})
	if blown == nil {
		t.Fatal("want the panic handed back")
	}

	if !strings.Contains(
		blown.Error(),
		WHY,
	) {
		t.Fatalf(
			"want the panic's own words, got %v",
			blown,
		)
	}

	quiet := guard.Contained(func() {
		// Empty
	})
	if quiet != nil {
		t.Fatalf(
			"want nothing for work that did not raise, got %v",
			quiet,
		)
	}
}

// TestContained_KeepsAnErrorMatchable records that a panic carrying an error
// stays matchable through the recovery, as WithRecover's does.
//
// Revisions:
//   - 2026-09-20 12:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestContained_KeepsAnErrorMatchable(t *testing.T) {
	var (
		sentinel = errors.New("a host's own failure")
		blown    = guard.Contained(func() {
			panic(sentinel)
		})
	)

	if !errors.Is(
		blown,
		sentinel,
	) {
		t.Fatalf(
			"want the panic's error still matchable, got %v",
			blown,
		)
	}
}

// TestWithRecover_NilResult proves a nil result pointer is never dereferenced
// and fn still runs: the error side keeps working whether fn returns or panics.
//
// Revisions:
//   - 2026-11-03 00:00: initial creation, for B055 (C48); replaces the nil pointer table
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_NilResult(t *testing.T) {
	type _Body func() (int, error)

	var (
		returns _Body = func() (int, error) {
			return VALUE, ErrBoom
		}
		panicsError _Body = func() (int, error) {
			panic(ErrBoom)
		}
		panicsText _Body = func() (int, error) {
			panic(TEXT)
		}
	)

	tests := []struct {
		name string
		body _Body
		want string
	}{
		{
			"returns",
			returns,
			"boom",
		},
		{
			"panics error",
			panicsError,
			"boom",
		},
		{
			"panics text",
			panicsText,
			TEXT,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var err error

				guard.WithRecover[int](
					nil,
					&err,
					tt.body,
				)

				if err == nil || !strings.Contains(
					err.Error(),
					tt.want,
				) {
					t.Fatalf(
						"got %v, want an error with %q",
						err,
						tt.want,
					)
				}
			},
		)
	}
}

// TestWithRecover_NilErrPanicsAtEntry proves a nil err pointer panics with the
// documented text before fn runs, whether or not result is nil.
//
// Revisions:
//   - 2026-11-03 00:00: initial creation, for C48
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWithRecover_NilErrPanicsAtEntry(t *testing.T) {
	const WANT = "guard.WithRecover: nil err pointer"

	var got int

	tests := []struct {
		name   string
		result *int
	}{
		{
			"result present",
			&got,
		},
		{
			"result nil",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var called bool

				defer func() {
					raised := recover()
					if raised != WANT {
						t.Fatalf(
							"got panic %v, want %q",
							raised,
							WANT,
						)
					}

					if called {
						t.Fatal("fn ran before the nil err panic")
					}
				}()

				guard.WithRecover(
					tt.result,
					nil,
					func() (int, error) {
						called = true

						return VALUE, nil
					},
				)
			},
		)
	}
}
