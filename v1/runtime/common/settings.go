package common

import (
	"log/slog"
	"sync/atomic"
)

// Settings is what a run is given besides its context and its thread: whatever
// watches it, and where what it prints goes.
//
// A struct rather than values on the context, so what a run depends on is in
// the call that starts one, and the context carries only cancellation.
// Exported fields, because the package that starts a run builds one. Each may
// be left out: no reporter is a run nothing watches, no logger prints to
// slog's default, and no Watch is a run that does not watch memory.
//
// Logger is an atomic pointer, so a host may change it while a run starts
// (Q211). Set it with settings.Logger.Store(logger). A struct literal cannot
// set it, because an atomic value is not copied in.
//
// Watch, when set, starts the memory watch (package memwatch) with the run and
// stops it when the run is over. A trip ends the run the way assert does, with
// memwatch.ErrLimit as its outcome.
//
// Revisions:
//   - 2026-11-02 22:32: Watch, the opt-in memory watch (C27)
//   - 2026-11-06 00:45: Logger is an atomic.Pointer[slog.Logger], set with Store
//     (C100a, B171, Q211)
//   - 2026-11-07 19:14: moved to package common (C91w4)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
type Settings struct {
	Reporter Reporter
	Logger   atomic.Pointer[slog.Logger]
	Watch    *MemoryWatch
}
