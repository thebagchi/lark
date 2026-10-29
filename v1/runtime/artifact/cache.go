package artifact

import (
	"fmt"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Cache is what the top level of one pure unit produced, kept by the first
// run that built it for every later run of the artifact.
//
// gate is a channel of one slot rather than a sync.Once or a mutex, because a
// run that waits for another run to build the unit must still stop when its
// own context does. kept is nil until a run has an outcome worth keeping, and
// is read without the gate once it is set.
type _Cache struct {
	gate chan struct{}
	kept atomic.Pointer[_Kept]
}

// _Kept is the outcome of a pure unit's top level: its frozen globals, or the
// error that stopped it.
type _Kept struct {
	globals starlark.StringDict
	err     error
}

// _NewCache is a cache that holds nothing yet.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func _NewCache() *_Cache {
	return &_Cache{
		gate: make(
			chan struct{},
			1,
		),
	}
}

// _Globals is what the unit's top level produced, built on thread by the first
// run that asks.
//
// One build at a time, behind the gate, so two first runs build the unit once
// and share the result. A run that waits there stops with ErrCancelled when its
// context is done, as a run stopped anywhere else does.
//
// An error is kept as well as globals: a top level that calls nothing has no
// input, so every later run would fail the same way. The one exception is a
// build that fails while its run is stopped. That stop is the run's, not the
// unit's, so nothing is kept and the next run builds again.
//
// Returns ErrNoRun for a thread with no run, ErrCancelled when the run stops
// while it waits, and otherwise what build returned or what a run kept.
//
// Revisions:
//   - 2026-10-28 23:34: initial creation
func (c *_Cache) _Globals(
	thread *starlark.Thread,
	build func() (starlark.StringDict, error),
) (starlark.StringDict, error) {
	kept := c.kept.Load()
	if kept != nil {
		return kept.globals, kept.err
	}

	ctx, err := scheduler.Context(thread)
	if err != nil {
		return nil, err
	}

	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf(
			"%w: %w",
			scheduler.ErrCancelled,
			ctx.Err(),
		)
	}

	defer func() {
		<-c.gate
	}()

	// Another run may have built the unit while this one waited.
	kept = c.kept.Load()
	if kept != nil {
		return kept.globals, kept.err
	}

	// A failure while the run is stopped is the run's, not the unit's.
	globals, err := build()
	if err != nil && ctx.Err() != nil {
		return nil, err
	}

	c.kept.Store(&_Kept{
		globals: globals,
		err:     err,
	})

	return globals, err
}
