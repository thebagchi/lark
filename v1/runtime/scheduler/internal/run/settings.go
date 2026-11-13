package run

import (
	"log/slog"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// Logging is where a run with these settings prints: its logger, or slog's
// default when it names none.
//
// Revisions:
//   - 2026-10-03 08:30: initial creation, replacing the logger a context carried
//   - 2026-11-06 00:02: nil settings print to slog's default (C100a, B170)
//   - 2026-11-06 00:45: loads Logger once, so the nil check and the use see one
//     logger (C100a, B171, Q211)
//   - 2026-11-07 19:14: a function of *common.Settings, since the type moved
//     to common (C91w4, Q226)
//   - 2026-11-10 00:25: renamed from _Logging, as a test in tests/ needs it (C120a, Q314)
func Logging(settings *common.Settings) *slog.Logger {
	if settings == nil {
		return slog.Default()
	}

	logger := settings.Logger.Load()
	if logger == nil {
		return slog.Default()
	}

	return logger
}
