package tests

import (
	"context"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// TestCancelOn_ReleasesAgain checks the release of the watch Arm returns
// can be called a second time and returns at once, whether the context was
// cancelled before the first or never was.
//
// Revisions:
//   - 2026-10-03 23:37: initial creation
//   - 2026-10-28 22:36: releases through the watch _Arm now returns
//   - 2026-11-08 23:21: moved to the external package, through the finish function
//     of run.Begin, which releases the watch (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCancelOn_ReleasesAgain(t *testing.T) {
	cases := []struct {
		name      string
		cancelled bool
	}{
		{
			"never cancelled",
			false,
		},
		{
			"cancelled first",
			true,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				ctx, stop := context.WithCancel(t.Context())
				defer stop()

				finish := run.Begin(
					ctx,
					&starlark.Thread{},
					common.ENTRY,
					nil,
				)

				if item.cancelled {
					stop()
				}

				released := make(chan error)

				go func() {
					var (
						first  = finish(nil)
						second = finish(nil)
					)
					if first != nil {
						released <- first

						return
					}

					released <- second
				}()

				select {
				case err := <-released:
					if err != nil {
						t.Fatalf(
							"finish: %v",
							err,
						)
					}
				case <-time.After(CANCEL_LIMIT):
					t.Fatal("a second finish did not return")
				}
			},
		)
	}
}
