package guard_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/guard"
)

var ErrBoom = errors.New("guard: boom")

// WHY is the text a contained panic carries, so a test can look for it.
const WHY = "a host's callback blew up"

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
func TestContained_HandsBackThePanic(t *testing.T) {
	blown := guard.Contained(func() {
		panic(WHY)
	})

	if blown == nil {
		t.Fatal("want the panic handed back")
	}

	if !strings.Contains(blown.Error(), WHY) {
		t.Fatalf("want the panic's own words, got %v", blown)
	}

	quiet := guard.Contained(func() {})
	if quiet != nil {
		t.Fatalf("want nothing for work that did not raise, got %v", quiet)
	}
}

// TestContained_KeepsAnErrorMatchable records that a panic carrying an error
// stays matchable through the recovery, as WithRecover's does.
//
// Revisions:
//   - 2026-09-20 12:00: initial creation
func TestContained_KeepsAnErrorMatchable(t *testing.T) {
	sentinel := errors.New("a host's own failure")

	blown := guard.Contained(func() {
		panic(sentinel)
	})

	if !errors.Is(blown, sentinel) {
		t.Fatalf("want the panic's error still matchable, got %v", blown)
	}
}
