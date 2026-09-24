package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"go.starlark.net/starlark"
)

const (
	// CEILING is how much memory one run may hold through this library at
	// once, until a host chooses otherwise.
	//
	// This is the one number the design needs, and it is the answerable one.
	// "How big may a file be" and "how long may a line be" are policies nobody
	// can choose for someone else; "how much memory may a script use" is a
	// question the host running it already knows the answer to.
	CEILING = 256 << 20
)

// ErrMemory is returned for an allocation that would take a run past what it
// may hold.
var ErrMemory = errors.New("past the memory this run may use")

// Budget is how much memory one run may hold through this library.
//
// Counted rather than observed. The Go heap is process wide, so a run watching
// it would be failed for memory another run in the same process allocated, and
// a library that fails work for its neighbour's reasons is worse than one that
// does not try. What is counted here is what this library allocated on a
// script's behalf, which is the whole of what it can honestly answer for.
//
// What this does not bound: a value handed to a script is credited back when
// it is handed over, because from then on Go's collector decides when it goes.
// So this bounds one allocation, and every allocation in flight at once - not
// the total a script has accumulated and still holds.
type Budget struct {
	ceiling int64
	held    atomic.Int64
}

// NewBudget is a budget with ceiling bytes to give out.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
func NewBudget(ceiling int64) *Budget {
	return &Budget{ceiling: ceiling}
}

// Charge reserves size bytes, or refuses without reserving anything.
//
// Safe from any thread of a run: several may be reading at once, and the
// ceiling belongs to the run rather than to each thread.
//
// Returns ErrMemory naming what was asked for and what was left. Never panics.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
func (b *Budget) Charge(size int64) error {
	if size <= 0 {
		return nil
	}

	for {
		held := b.held.Load()

		if held+size > b.ceiling {
			return fmt.Errorf("asked for %d bytes with %d of %d left: %w",
				size, b.ceiling-held, b.ceiling, ErrMemory)
		}

		if b.held.CompareAndSwap(held, held+size) {
			return nil
		}
	}
}

// Credit gives size bytes back.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
func (b *Budget) Credit(size int64) {
	if size <= 0 {
		return
	}

	b.held.Add(-size)
}

// Left is how much the run may still ask for.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
func (b *Budget) Left() int64 {
	return b.ceiling - b.held.Load()
}

// Held is what this run has reserved now.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
func (b *Budget) Held() int64 {
	return b.held.Load()
}

// Allowance is the budget the run on thread is working to, or a fresh one at
// the default ceiling when the thread has no run.
//
// Per run rather than per thread, because the threads of one run share the
// memory of the process they are in.
//
// A thread with no run gets a budget of its own rather than an error. Nothing
// outside a run can be failed by one, and a host evaluating an expression on a
// bare thread should not be told about runs by a call that reads a file.
//
// Revisions:
//   - 2026-09-24 22:52: initial creation
//   - 2026-09-24 23:04: a thread with no run gets one, rather than an error
func Allowance(thread *starlark.Thread) *Budget {
	locals, err := _Of(thread)
	if err != nil {
		return NewBudget(CEILING)
	}

	return locals.run.budget
}

// _Ceiling is the context key a chosen ceiling is carried under. Its own type,
// so nothing else can collide with it.
type _Ceiling struct{}

// Allowing is ctx carrying a ceiling other than CEILING, for the run started
// under it.
//
// Carried on the context for the same reason the reporter is: a run is made
// inside a call this package owns, and a host has no other way to reach it.
// Chosen before the run starts and never after, because a ceiling a script
// could raise partway through is not a ceiling.
//
// Revisions:
//   - 2026-09-24 23:10: initial creation
func Allowing(ctx context.Context, ceiling int64) context.Context {
	return context.WithValue(ctx, _Ceiling{}, ceiling)
}

// _Chosen is the ceiling ctx carries, or the default when it carries none.
//
// Revisions:
//   - 2026-09-24 23:10: initial creation
func _Chosen(ctx context.Context) int64 {
	held, ok := ctx.Value(_Ceiling{}).(int64)
	if !ok || held <= 0 {
		return CEILING
	}

	return held
}
