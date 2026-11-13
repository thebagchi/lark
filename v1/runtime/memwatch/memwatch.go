// Package memwatch ends a run before the machine it runs on is out of memory.
//
// The watch is opt-in and off by default. A host turns it on with the run
// option WithMemoryWatch (package runtime) or the lark flag -memory. It
// reads, every 100 ms, how much memory is in use on the machine
// (/proc/meminfo, MemTotal less MemAvailable) and in this process's cgroup v2
// (memory.current less inactive_file, over memory.max), takes the one with the
// larger share, and trips at 95 percent. Any process's use counts: the kernel
// kills on the whole machine's or cgroup's memory, not on lark's share.
//
// A trip ends the run the way assert does: the run's outcome is
// ErrLimit, "memory limit reached". A Go builtin that allocates in a
// loop never looks at the run's context, so the run may not end. Then, 500 ms
// after the trip, the host's abandon function runs; the lark command prints one
// line and exits 5, as it does when the run stops by itself inside the grace. A
// library host passes nil and keeps its own policy.
//
// Where the system offers no /proc/meminfo or no cgroup v2, Check returns
// ErrUnavailable naming the file it could not read, and a run that asked for
// the watch is refused: the watch never silently does nothing. Under WSL2 the
// host is the VM, so "the machine" is the VM's memory.
//
// Layout, as CLAUDE.md asks: this package holds functions and sentinel errors
// only, and each function is one line; the types are in package common, which
// holds data only; the behavior is in the internal package watch.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-02 23:47: the exit code in this text is 5 (C27)
//   - 2026-11-03 00:40: Room, Fit and System refuse a size that does not fit (C46)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
package memwatch

import (
	"context"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch/internal/watch"
)

// ErrLimit is the outcome of a run the watch ended. The text is
// "memory limit reached".
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrLimitReached (C121, Q349)
var (
	ErrLimit = watch.ErrLimit
)

// ErrUnavailable is returned by Check when the system does not offer what the
// watch reads. The text names the file it could not read.
//
// Revisions:
//   - 2026-11-02 23:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrUnavailable = watch.ErrUnavailable
)

// ErrRoom is returned by Room and Fit for a size that does not fit the free
// memory. The text is "not enough free memory".
//
// Revisions:
//   - 2026-11-03 22:33: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 17:15: renamed from ErrNoRoom (C121, Q349)
var (
	ErrRoom = watch.ErrRoom
)

// Start begins watching and returns the function that stops the watch. A nil w
// is no watch: nothing starts and nothing is allocated. trip is called once,
// with ErrLimit, when memory in use reaches the limit.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func Start(
	ctx context.Context,
	w *common.MemoryWatch,
	trip func(error),
) func() {
	return watch.Start(
		ctx,
		w,
		trip,
	)
}

// Check returns nil when the system offers what the watch reads, and otherwise
// an error wrapping ErrUnavailable that names what it could not read.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Check() error {
	return watch.Check(
		watch.MEMINFO,
		watch.CGROUP,
		watch.SELF,
	)
}

// Room refuses a script-given size of bytes that does not fit the free memory,
// with an error wrapping ErrRoom, before the caller allocates. It does not
// need the watch to be on. See Fit for the rule.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Room(
	who string,
	size uint64,
) error {
	return watch.Room(
		who,
		size,
	)
}

// Fit is Room with the reading given. A size below common.ROOM_SKIP is not
// read; where read fails, a size above common.ROOM_CEILING is refused.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
func Fit(
	who string,
	size uint64,
	read func() (*common.MemoryReading, error),
) error {
	return watch.Fit(
		who,
		size,
		read,
	)
}

// System returns the reading of the tighter of the machine and this process's
// cgroup: one callable unit, with no watch needed.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
func System() func() (*common.MemoryReading, error) {
	return watch.System()
}
