// Package walk holds the behavior of the shared walker. Its callers are in
// the package trail, which forwards to it. See the package trail for what the
// walker is for.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package walk

import (
	"context"
	"errors"
	"reflect"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// ErrFatal marks an error that ends the run, the way assert does.
//
// An error wraps it when the script cannot continue and no other thread can
// be trusted either. The scheduler asks errors.Is(err, ErrFatal) in one place,
// where a thread ends.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrFatal = errors.New("ends the run")
)

// ErrCyclic is returned for a container that holds itself. It is an
// ordinary thread error, like the one fail() raises.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: renamed from ErrSelfContaining (C121, Q349)
var (
	ErrCyclic = errors.New("cycle in value")
)

// ErrDeep is returned for a value nested deeper than common.DEPTH. It wraps
// ErrFatal.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
var (
	ErrDeep error = new(_DeepError)
)

// _DeepError is the type of ErrDeep. It exists so the error reads as one
// sentence and still wraps ErrFatal: a join of two errors prints two lines.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
type _DeepError struct{}

// Error returns the text of the error.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func (e *_DeepError) Error() string {
	return "value nested too deep"
}

// Unwrap returns ErrFatal, so errors.Is(err, ErrFatal) holds.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func (e *_DeepError) Unwrap() error {
	return ErrFatal
}

// New returns a Trail that refuses a cycle and a depth past common.DEPTH, and
// looks at ctx every common.POLL elements.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func New(ctx context.Context) *common.Trail {
	made := _Made(ctx)
	made.Cycles = true
	made.Path = made.Inline[:0]

	return made
}

// Bounded returns a Trail that refuses a depth past common.DEPTH and looks at
// ctx every common.POLL elements, and does not look for a cycle.
//
// For a walk that copes with a cycle itself, because a cycle is data to it: it
// keeps a set of what it has seen. The depth count costs one integer.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Bounded(ctx context.Context) *common.Trail {
	return _Made(ctx)
}

// _Made returns a Trail that looks at ctx and has no other setting.
//
// A context that can never be cancelled, such as context.Background, leaves
// Err nil, so the walk makes no call for it.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func _Made(ctx context.Context) *common.Trail {
	made := new(common.Trail)

	if ctx != nil && ctx.Done() != nil {
		made.Err = ctx.Err
	}

	return made
}

// Enter counts one element, then records that the walk goes into container.
//
// It returns ErrCyclic when container is already on the path and the
// Trail looks for cycles, ErrDeep when the walk would pass common.DEPTH,
// and the context's error when a look at the context finds it done. After an
// error nothing is recorded, so the caller does not call Leave.
//
// container is a pointer, or nil for a tuple. A slice cannot be compared, and
// a tuple cannot hold itself. Any other kind of value (a slice, a map, a
// function, a struct) is treated as nil: it has no identity to compare, so
// only the depth limit guards it. A nil trail is refused with a plain error (answered by
// Tick): a walk with no trail has no bound.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-04 19:30: a nil trail is ErrFatal; a container that is not a
//     pointer has no identity, so it is nil, not a panic (C87)
//   - 2026-11-04 19:30: a nil trail is ErrNoTrail, from Tick (C87 fix round 1)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrCyclic and ErrDeep, formerly ErrSelfContaining and ErrTooDeep
//     (C121, Q349)
func Enter(
	trail *common.Trail,
	container any,
) error {
	err := Tick(trail)
	if err != nil {
		return err
	}

	container = _Identity(container)

	if trail.Cycles && _OnPath(
		trail,
		container,
	) {
		return ErrCyclic
	}

	if trail.Depth >= common.DEPTH {
		return ErrDeep
	}

	trail.Depth++

	if trail.Cycles {
		trail.Path = append(
			trail.Path,
			container,
		)
	}

	return nil
}

// Leave records that the walk came out of the container it last entered.
//
// A Leave with nothing entered, or on a nil trail, does nothing.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-04 19:30: a nil trail is a no-op (C87)
func Leave(trail *common.Trail) {
	if trail == nil || trail.Depth == 0 {
		return
	}

	trail.Depth--

	last := len(trail.Path) - 1
	if last < 0 {
		return
	}

	trail.Path[last] = nil
	trail.Path = trail.Path[:last]
}

// Tick counts one element. Every common.POLL elements it looks at the context
// and returns its error when the context is done. A nil trail is refused with a plain
// error, as for Enter.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-04 19:30: a nil trail is ErrFatal (C87)
//   - 2026-11-04 19:30: a nil trail is ErrNoTrail, the guard in Enter is
//     Tick's (C87 fix round 1, Q112)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
func Tick(trail *common.Trail) error {
	if trail == nil {
		return errors.New("walk has no trail")
	}

	trail.Count++
	if trail.Count < common.POLL {
		return nil
	}

	trail.Count = 0

	if trail.Err == nil {
		return nil
	}

	return trail.Err()
}

// _Identity returns container when it is a pointer, and nil for anything else.
//
// A pointer is the one kind that == can always compare. A slice, a map, a
// function, or a struct holding one, makes == panic when two of that type meet.
// Cost: one kind test, with no allocation.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Identity(container any) any {
	if container == nil ||
		reflect.TypeOf(container).Kind() != reflect.Pointer {
		return nil
	}

	return container
}

// _OnPath reports whether container is a container the walk is inside.
//
// It reads from the newest entry, because a cycle usually closes near the
// walk's position. Cost: up to common.DEPTH comparisons, once for each
// container entered.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func _OnPath(
	trail *common.Trail,
	container any,
) bool {
	if container == nil {
		return false
	}

	for index := len(trail.Path) - 1; index >= 0; index-- {
		if trail.Path[index] == container {
			return true
		}
	}

	return false
}
