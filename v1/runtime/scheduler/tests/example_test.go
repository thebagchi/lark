package tests

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ExampleContext shows a builtin that waits on a channel and on the context
// scheduler.Context gives it, so cancelling the run ends the wait at once.
//
// Revisions:
//   - 2026-11-08 02:09: initial creation (C76, Q236)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func ExampleContext() {
	var (
		never = make(chan struct{})
		wait  = starlark.NewBuiltin(
			"wait",
			func(
				thread *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				ctx, err := scheduler.Context(thread)
				if err != nil {
					return nil, err
				}

				select {
				case <-never:
					return starlark.None, nil

				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		)
		ctx, cancel = context.WithCancel(context.Background())
		thread      = &starlark.Thread{
			Name: "spine",
		}
		finish = scheduler.Begin(
			ctx,
			thread,
			common.ENTRY,
			nil,
		)
	)

	go cancel()

	_, err := starlark.Call(
		thread,
		wait,
		nil,
		nil,
	)
	_ = finish(err)

	fmt.Println(errors.Is(
		err,
		context.Canceled,
	))
	// Output: true
}
