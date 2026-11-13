package event

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/event/internal/latch"
	"github.com/thebagchi/lark/v1/runtime/plugin/event/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Of is this run's events, made on first use.
//
// Revisions:
//   - 2026-09-30 21:01: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses latch.Events and latch.Latch, which moved to internal/latch
//     (C120b, Q315)
func _Of(thread *starlark.Thread) (*latch.Events, error) {
	return scheduler.Shared(
		thread,
		spec.EVENTS,
		func() *latch.Events {
			return &latch.Events{
				Held: map[string]*latch.Latch{},
			}
		},
	)
}
