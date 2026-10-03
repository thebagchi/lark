package scheduler

import "log/slog"

// Settings is what a run is given besides its context and its thread: whatever
// watches it, where what it prints goes, and how much memory it may hold.
//
// A struct rather than values on the context, so what a run depends on is in
// the call that starts one, and the context carries only cancellation.
// Exported fields, because the package that starts a run builds one. Each may
// be left out: no reporter is a run nothing watches, no logger prints to
// slog's default, and no ceiling is CEILING.
type Settings struct {
	Reporter Reporter
	Logger   *slog.Logger
	Ceiling  int64
}

// _Logging is where a run with these settings prints: its logger, or slog's
// default when it names none.
//
// Revisions:
//   - 2026-10-03 08:30: initial creation, replacing the logger a context carried
func (s *Settings) _Logging() *slog.Logger {
	if s.Logger == nil {
		return slog.Default()
	}

	return s.Logger
}

// _Chosen is the ceiling a run with these settings is held to: its own, or
// CEILING when it names none.
//
// Revisions:
//   - 2026-10-03 08:30: initial creation, replacing the ceiling a context carried
func (s *Settings) _Chosen() int64 {
	if s.Ceiling <= 0 {
		return CEILING
	}

	return s.Ceiling
}
