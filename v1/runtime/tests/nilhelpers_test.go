package tests

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Last is the last value of a call's results when it is an error, so a table
// row can wrap a call that returns anything beside an error.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 3)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Last(results ...any) error {
	if len(results) == 0 {
		return nil
	}

	err, _ := results[len(results)-1].(error)

	return err
}

// TestNilSweepHelpers proves the helpers a plugin author calls - unpack,
// plugin and the scheduler's - answer a nil or zero input instead of
// panicking, with the error the table names where the helper returns one. It
// is the second half of TestNilSweep, split to keep each table readable.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: the Listener cases moved to remote/internal/host (C91w8c)
//   - 2026-11-07 20:07: updated for the move of Handle, Evaluation and Hold to common (C91w5)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun, formerly ErrNoRun; uses Eval, formerly Evaluate (C121,
//     Q349, Q350)
//   - 2026-11-12 18:58: uses Fork, formerly Beside (C129, Q396)
func TestNilSweepHelpers(t *testing.T) {
	var (
		ctx        = context.Background()
		nilContext context.Context
		empty      = func() *starlark.Thread {
			return new(starlark.Thread)
		}
		noFunction *starlark.Builtin
		builtin    = starlark.NewBuiltin(
			"f",
			nil,
		)
		oneString  = starlark.Tuple{starlark.String("a")}
		evaluation common.Evaluation
		handle     common.Handle
		body       = func(*starlark.Thread) (starlark.Value, error) {
			return starlark.None, nil
		}
	)

	cases := []struct {
		name string
		call func() error
		want error
	}{
		{
			"unpack.ShortPath nil",
			func() error {
				return unpack.ShortPath(nil)
			},
			nil,
		},
		{
			"unpack.Handover nil",
			func() error {
				_ = unpack.Handover(nil)
				return nil
			},
			nil,
		},
		{
			"unpack.Padded nil number",
			func() error {
				_, _ = unpack.Padded(
					nil,
					10,
					3,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.Padded base 0",
			func() error {
				_, _ = unpack.Padded(
					big.NewInt(5),
					0,
					3,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.Padded base 99",
			func() error {
				_, _ = unpack.Padded(
					big.NewInt(5),
					99,
					3,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.Padded negative width",
			func() error {
				_, _ = unpack.Padded(
					big.NewInt(5),
					10,
					-3,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.CheckPadding zero group",
			func() error {
				_, _ = unpack.CheckPadding(
					"abc",
					0,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.CheckPadding negative group",
			func() error {
				_, _ = unpack.CheckPadding(
					"abc",
					-1,
				)

				return nil
			},
			nil,
		},
		{
			"unpack.Positional nil builtin",
			func() error {
				return unpack.Positional(
					noFunction,
					nil,
					nil,
					1,
				)
			},
			nil,
		},
		{
			"unpack.StringAt nil builtin",
			func() error {
				return _Last(unpack.StringAt(
					noFunction,
					nil,
					0,
				))
			},
			unpack.ErrRange,
		},
		{
			"unpack.StringAt negative index",
			func() error {
				return _Last(unpack.StringAt(
					builtin,
					oneString,
					-1,
				))
			},
			unpack.ErrRange,
		},
		{
			"unpack.StringAt index past end",
			func() error {
				return _Last(unpack.StringAt(
					builtin,
					nil,
					5,
				))
			},
			unpack.ErrRange,
		},
		{
			"unpack.StringAt nil value",
			func() error {
				return _Last(unpack.StringAt(
					builtin,
					starlark.Tuple{nil},
					0,
				))
			},
			nil,
		},
		{
			"unpack.CallableAt nil builtin",
			func() error {
				return _Last(unpack.CallableAt(
					noFunction,
					nil,
					0,
				))
			},
			unpack.ErrRange,
		},
		{
			"unpack.CallableAt negative index",
			func() error {
				return _Last(unpack.CallableAt(
					builtin,
					oneString,
					-1,
				))
			},
			unpack.ErrRange,
		},
		{
			"unpack.CallableAt nil value",
			func() error {
				return _Last(unpack.CallableAt(
					builtin,
					starlark.Tuple{nil},
					0,
				))
			},
			nil,
		},
		{
			"unpack.Data nil builtin",
			func() error {
				return _Last(unpack.Data(
					noFunction,
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"unpack.Data nil value",
			func() error {
				return _Last(unpack.Data(
					builtin,
					starlark.Tuple{nil},
					nil,
				))
			},
			unpack.ErrData,
		},
		{
			"unpack.Data short keyword",
			func() error {
				return _Last(unpack.Data(
					builtin,
					nil,
					[]starlark.Tuple{oneString},
				))
			},
			nil,
		},
		{
			"unpack.Bytes nil value",
			func() error {
				return _Last(unpack.Bytes(
					"x",
					nil,
				))
			},
			unpack.ErrData,
		},
		{
			"unpack.Text nil builtin",
			func() error {
				return _Last(unpack.Text(
					noFunction,
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"unpack.Counted nil builtin",
			func() error {
				return _Last(unpack.Counted(
					noFunction,
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"unpack.Counted nil value",
			func() error {
				return _Last(unpack.Counted(
					builtin,
					starlark.Tuple{nil, nil},
					nil,
				))
			},
			nil,
		},
		{
			"unpack.Unprefixed empty",
			func() error {
				_ = unpack.Unprefixed(
					"",
					"",
				)

				return nil
			},
			nil,
		},
		{
			"unpack.FromProto nil",
			func() error {
				return _Last(unpack.FromProto(nil))
			},
			nil,
		},
		{
			"unpack.DataView nil builtin",
			func() error {
				return _Last(unpack.DataView(
					noFunction,
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"unpack.DataView nil value",
			func() error {
				return _Last(unpack.DataView(
					builtin,
					starlark.Tuple{nil},
					nil,
				))
			},
			unpack.ErrData,
		},
		{
			"unpack.View nil value",
			func() error {
				return _Last(unpack.View(
					"x",
					nil,
				))
			},
			unpack.ErrData,
		},
		{
			"plugin.Register nil",
			func() error {
				plugin.Register(nil)
				return nil
			},
			nil,
		},
		{
			"scheduler.Begin nil thread",
			func() error {
				_ = scheduler.Begin(
					ctx,
					nil,
					"x",
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Begin nil context",
			func() error {
				_ = scheduler.Begin(
					nilContext,
					empty(),
					"x",
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Context nil thread",
			func() error {
				return _Last(scheduler.Context(nil))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Context zero thread",
			func() error {
				return _Last(scheduler.Context(empty()))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Ambient nil thread",
			func() error {
				_ = scheduler.Ambient(nil)
				return nil
			},
			nil,
		},
		{
			"scheduler.Fail nil thread",
			func() error {
				_ = scheduler.Fail(
					nil,
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.AttemptOf nil thread",
			func() error {
				_ = scheduler.AttemptOf(nil)
				return nil
			},
			nil,
		},
		{
			"scheduler.Shared nil thread",
			func() error {
				return _Last(scheduler.Shared[int](
					nil,
					"k",
					func() int {
						return 1
					},
				))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Hidden nil",
			func() error {
				_, _ = scheduler.Hidden(
					nil,
					"x",
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Hidden nil pair",
			func() error {
				_, _ = scheduler.Hidden(
					[]starlark.Tuple{nil},
					"x",
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Lock nil thread",
			func() error {
				return _Last(scheduler.Lock(
					nil,
					"n",
				))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Updating nil thread",
			func() error {
				return _Last(scheduler.Updating(nil))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Poll nil thread",
			func() error {
				scheduler.Poll(nil)
				return nil
			},
			nil,
		},
		{
			"scheduler.Polled nil",
			func() error {
				_ = scheduler.Polled(nil)
				return nil
			},
			nil,
		},
		{
			"scheduler.Duration nil",
			func() error {
				return _Last(scheduler.Duration(nil))
			},
			scheduler.ErrDuration,
		},
		{
			"scheduler.Fork nil thread",
			func() error {
				return _Last(scheduler.Fork(
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"scheduler.Named nil target",
			func() error {
				_ = scheduler.Named(
					nil,
					"",
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Wait nil thread",
			func() error {
				return _Last(scheduler.Wait(
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"scheduler.Wait nil handle",
			func() error {
				return _Last(scheduler.Wait(
					empty(),
					handle,
				))
			},
			nil,
		},
		{
			"scheduler.Settle nil thread and handle",
			func() error {
				scheduler.Settle(
					nil,
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Settle nil handle",
			func() error {
				scheduler.Settle(
					empty(),
					handle,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Eval nil context",
			func() error {
				return _Last(scheduler.Eval(
					nilContext,
					"x",
					body,
					nil,
				))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.Eval nil body",
			func() error {
				_, _ = scheduler.Eval(
					ctx,
					"x",
					nil,
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Open nil thread",
			func() error {
				scheduler.Open(
					nil,
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Close nil thread",
			func() error {
				scheduler.Close(
					nil,
					"x",
					nil,
				)

				return nil
			},
			nil,
		},
		{
			"scheduler.Within nil thread",
			func() error {
				return _Last(scheduler.Within(
					nil,
					nil,
				))
			},
			nil,
		},
		{
			"scheduler.EvaluationOf nil thread",
			func() error {
				return _Last(scheduler.EvaluationOf(nil))
			},
			scheduler.ErrRun,
		},
		{
			"scheduler.SharedIn nil evaluation",
			func() error {
				return _Last(scheduler.SharedIn[int](
					evaluation,
					"k",
					func() int {
						return 1
					},
				))
			},
			scheduler.ErrRun,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				_Survives(
					t,
					func() {
						err = tc.call()
					},
				)

				if tc.want != nil && !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"want %v, got %v",
						tc.want,
						err,
					)
				}
			},
		)
	}
}
