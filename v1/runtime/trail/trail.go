// Package trail is the one walker that every Go walk over a script value
// uses, so that no script can end the host with a value it builds.
//
// A walk is a Go function that recurses on the elements of a list, a dict, a
// tuple or a set. A script builds a list that holds itself, or a list nested
// a million deep, in a few lines. A Go stack overflow is a fatal error, not a
// panic, so nothing can recover it. The walker does three things:
//
//   - a cycle check: a container already on the current path is an error,
//     found by pointer identity, so a container reached twice by two routes is
//     not a cycle;
//   - a depth limit, common.DEPTH, the number protobuf and encoding/json use;
//   - a look at the context every common.POLL elements, so a long walk stops
//     when the run is cancelled.
//
// A cycle (ErrCyclic) is an ordinary error: it ends one thread, join
// reports it, and a script that does not join carries on. A depth past the
// limit (ErrDeep) wraps ErrFatal and ends the run the way assert does; the
// scheduler asks errors.Is(err, ErrFatal) where a thread ends, so no walk
// calls the scheduler.
//
// Usage, in a recursive walk:
//
//	err := trail.Enter(path, container)
//	if err != nil {
//		return err
//	}
//	defer trail.Leave(path)
//
// and trail.Tick(path) for each element that is not a container.
//
// Layout, as CLAUDE.md asks: this package holds functions and sentinel
// errors only, and each function is one line; the Trail type is in package
// common, which holds data only; the behavior is in the internal package
// walk.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 17:15: uses ErrCyclic and ErrDeep, formerly ErrSelfContaining and ErrTooDeep
//     (C121, Q349)
package trail

import (
	"context"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/trail/internal/walk"
)

// ErrFatal marks an error that ends the run, the way assert does.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrFatal = walk.ErrFatal
)

// ErrCyclic is returned for a container that holds itself. The text is
// "cycle in value". It is an ordinary thread error.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrSelfContaining (C121, Q349)
var (
	ErrCyclic = walk.ErrCyclic
)

// ErrDeep is returned for a value nested deeper than common.DEPTH. The text
// is "value nested too deep". It wraps ErrFatal.
//
// Revisions:
//   - 2026-11-02 16:21: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: renamed from ErrTooDeep (C121, Q349)
var (
	ErrDeep = walk.ErrDeep
)

// New returns a Trail that refuses a cycle and a depth past common.DEPTH, and
// looks at ctx every common.POLL elements.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func New(ctx context.Context) *common.Trail {
	return walk.New(ctx)
}

// Bounded returns a Trail that refuses a depth past common.DEPTH and looks at
// ctx every common.POLL elements, and leaves cycles to the walk. Use it where a
// cycle is data, for a walk that keeps a set of what it has seen.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Bounded(ctx context.Context) *common.Trail {
	return walk.Bounded(ctx)
}

// Enter counts an element and records that the walk goes into container, a
// pointer, or nil for a tuple. After an error the caller does not call Leave.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Enter(
	path *common.Trail,
	container any,
) error {
	return walk.Enter(
		path,
		container,
	)
}

// Leave records that the walk came out of the container it last entered.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Leave(path *common.Trail) {
	walk.Leave(path)
}

// Tick counts an element that is not a container, and looks at the context
// every common.POLL elements.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
func Tick(path *common.Trail) error {
	return walk.Tick(path)
}
