package tests

import (
	"log/slog"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestGuard_C100a_LoggerRace proves a host may change Settings.Logger while a
// run reads it, under -race.
//
// Case from Go-level breaker, finding scheduler-F71 (bug B171; Q211). The
// breaker wrote the field with a plain store; the field is now atomic.
//
// Revisions:
//   - 2026-11-06 00:45: initial creation (C100a)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-09 02:09: moved to the external package, through run.Begin,
//     which reads the logger (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuard_C100a_LoggerRace(t *testing.T) {
	const ROUNDS = 2000

	var (
		settings = new(common.Settings)
		group    sync.WaitGroup
	)

	group.Go(func() {
		for range ROUNDS {
			settings.Logger.Store(slog.Default())
		}
	})

	group.Go(func() {
		for range ROUNDS {
			var (
				finish = run.Begin(
					t.Context(),
					new(starlark.Thread),
					common.ENTRY,
					settings,
				)
				err = finish(nil)
			)
			if err != nil {
				t.Error(err)
			}
		}
	})

	group.Wait()
}
