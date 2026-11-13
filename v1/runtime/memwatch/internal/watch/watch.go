// Package watch holds the behavior of the memory watch. Its callers are in the
// package memwatch, which forwards to it. See the package memwatch for what the
// watch is for.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package watch

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// ErrLimit is what a tripped watch hands its caller. Its text is
// "memory limit reached", in the style go.starlark.net uses.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrLimitReached (C121, Q349)
var (
	ErrLimit = errors.New("memory limit reached")
)

// ErrUnavailable is returned when the watch is on and the system does not offer
// what it reads. The error names the file it could not read.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrUnavailable = errors.New("memory watch unavailable")
)

// ErrRoom is returned by Fit and Room for a size that does not fit the free
// memory. Its text is "not enough free memory", in the style go.starlark.net
// uses.
//
// Revisions:
//   - 2026-11-03 22:33: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrNoRoom (C121, Q349)
var (
	ErrRoom = errors.New("not enough free memory")
)

// Start reads memory every Interval until ctx ends or the returned function is
// called, and calls trip once, with ErrLimit, the first time the share
// in use reaches Percent. A nil w starts nothing and returns a function that
// does nothing.
//
// A reading that fails is skipped. A watch that ended the run for a file it
// could not read would be the failure it exists to prevent; Check is where an
// unreadable system is refused, before the run starts.
//
// After a trip, when w.Abandon is set and the returned function has not been
// called within Grace, Abandon runs with the same error. The caller therefore
// calls the returned function after the run has ended, not before: a run stuck
// in a Go builtin never ends, and that is the case Abandon is for. The
// returned function waits for the goroutine, so trip and Abandon are never
// called after it returns. Calling it twice is safe.
//
// A nil ctx is context.Background, and a nil trip is a trip that does nothing:
// the watch goroutine would otherwise call a nil function and end the host.
// Start returns only the stop function, so it cannot refuse.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-04 19:30: a nil context is Background and a nil trip does
//     nothing, so the goroutine never panics (C87)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func Start(
	ctx context.Context,
	w *common.MemoryWatch,
	trip func(error),
) func() {
	if w == nil {
		// A function that captures nothing is not allocated, so a run without a
		// watch pays nothing for the stop it is handed.
		return func() {
			// Nothing was started, so there is nothing to stop.
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if trip == nil {
		trip = func(error) {
			// The caller asked for no call at a trip.
		}
	}

	var (
		held         = _Filled(w)
		inner, abort = context.WithCancel(ctx)
		stopped      = make(chan struct{})
		done         = make(chan struct{})
		once         sync.Once
	)

	go func() {
		defer close(done)

		_Await(
			inner,
			held,
			trip,
			stopped,
		)
	}()

	return func() {
		once.Do(func() {
			close(stopped)
			abort()
		})

		<-done
	}
}

// _Filled returns w with each zero setting replaced by its default.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Filled(given *common.MemoryWatch) *common.MemoryWatch {
	w := *given

	if w.Percent <= 0 {
		w.Percent = common.MEMORY_PERCENT
	}

	if w.Interval <= 0 {
		w.Interval = common.WATCH_INTERVAL
	}

	if w.Grace <= 0 {
		w.Grace = common.WATCH_GRACE
	}

	if w.Source == nil {
		w.Source = System()
	}

	return &w
}

// _Await is the body of one watch's goroutine: watch, and after a trip wait out
// the grace for the run to end.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func _Await(
	ctx context.Context,
	w *common.MemoryWatch,
	trip func(error),
	stopped <-chan struct{},
) {
	if !_Watched(
		ctx,
		w,
		trip,
	) || w.Abandon == nil {
		return
	}

	select {
	case <-stopped:
	case <-time.After(w.Grace):
		w.Abandon(ErrLimit)
	}
}

// _Watched is the loop: wait, read, compare. It returns true when it tripped,
// and false when ctx ended first.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func _Watched(
	ctx context.Context,
	w *common.MemoryWatch,
	trip func(error),
) bool {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false

		case <-ticker.C:
		}

		if !_Over(
			w,
		) {
			continue
		}

		trip(ErrLimit)

		return true
	}
}

// _Over reads once and says whether the share in use has reached the limit.
// A failed reading, or a reading with no total, is not over.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Over(w *common.MemoryWatch) bool {
	reading, err := w.Source()
	if err != nil || reading == nil || reading.Total == 0 {
		return false
	}

	return 100*float64(reading.Used) >= w.Percent*float64(reading.Total)
}
