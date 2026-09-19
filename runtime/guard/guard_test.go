package guard_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/runtime/guard"
)

var ErrBoom = errors.New("guard: boom")

const (
	VALUE     = 7
	TEXT      = "not an error"
	RECOVERED = "recovered"
)

// TestWithRecover_PassesAValueThrough proves the ordinary path is untouched.
//
// Revisions:
//   - 2026-09-19 21:19: initial creation
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
		t.Fatalf("got %d, %v, want %d, nil", got, err, VALUE)
	}
}

// TestWithRecover_PassesAnErrorThrough proves a returned error is not touched.
//
// Revisions:
//   - 2026-09-19 21:19: initial creation
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

	if !errors.Is(err, ErrBoom) {
		t.Fatalf("got %v, want ErrBoom", err)
	}
}

// TestWithRecover_KeepsAPanickedErrorReachable proves errors.Is still works
// through the recovery, which is why a panic carrying an error is not wrapped
// with %v.
//
// Revisions:
//   - 2026-09-19 21:20: initial creation
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

	if !errors.Is(err, ErrBoom) {
		t.Fatalf("got %v, want ErrBoom reachable through the recovery", err)
	}

	if got != 0 {
		t.Fatalf("result is %d, want the zero value after a panic", got)
	}

	t.Logf("panicked error survives errors.Is: %v", err)
}

// TestWithRecover_WrapsAPanicThatIsNotAnError proves a panic of any other type
// still becomes an error rather than escaping.
//
// Revisions:
//   - 2026-09-19 21:21: initial creation
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

	if err == nil || !strings.Contains(err.Error(), TEXT) {
		t.Fatalf("got %v, want an error naming %q", err, TEXT)
	}

	if !strings.Contains(err.Error(), RECOVERED) {
		t.Fatalf("error does not say it was recovered: %v", err)
	}
}
