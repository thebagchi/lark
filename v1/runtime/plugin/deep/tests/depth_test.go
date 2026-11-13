package tests

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Nested builds a value with levels containers, one inside the next, of the
// kind named: a list, a tuple or a dict. The innermost holds nothing.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Nested(
	t *testing.T,
	kind string,
	levels int,
) starlark.Value {
	t.Helper()

	var value starlark.Value = starlark.NewList(nil)

	for range levels - 1 {
		switch kind {
		case "list":
			value = _Held(value)

		case "tuple":
			value = starlark.Tuple{value}

		case "dict":
			var (
				outer = starlark.NewDict(1)
				err   = outer.SetKey(
					starlark.String("k"),
					value,
				)
			)
			if err != nil {
				t.Fatal(err)
			}

			value = outer
		}
	}

	return value
}

// TestDepth_PastTheLimitIsTooDeep proves Copy and IsData refuse a value nested
// one container past common.DEPTH with trail.ErrDeep, for each kind of container,
// and accept one at the limit.
//
// Starting condition: common.DEPTH is 10,000.
// Success: the value at the limit passes; the value one deeper gives
// trail.ErrDeep, which wraps trail.ErrFatal.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestDepth_PastTheLimitIsTooDeep(t *testing.T) {
	for _, kind := range []string{"list", "tuple", "dict"} {
		t.Run(
			kind,
			func(t *testing.T) {
				var (
					ctx  = context.Background()
					atIt = _Nested(
						t,
						kind,
						common.DEPTH,
					)
					past = _Nested(
						t,
						kind,
						common.DEPTH+1,
					)
				)

				_, err := deep.Copy(
					ctx,
					atIt,
				)
				if err != nil {
					t.Fatalf(
						"Copy at the limit: %v",
						err,
					)
				}

				_, accepted, err := deep.IsData(
					ctx,
					atIt,
				)
				if err != nil || !accepted {
					t.Fatalf(
						"IsData at the limit: %v, %v",
						accepted,
						err,
					)
				}

				_, err = deep.Copy(
					ctx,
					past,
				)
				if !errors.Is(
					err,
					trail.ErrDeep,
				) || !errors.Is(
					err,
					trail.ErrFatal,
				) {
					t.Fatalf(
						"Copy past the limit = %v, want ErrDeep",
						err,
					)
				}

				_, _, err = deep.IsData(
					ctx,
					past,
				)
				if !errors.Is(
					err,
					trail.ErrDeep,
				) {
					t.Fatalf(
						"IsData past the limit = %v, want ErrDeep",
						err,
					)
				}
			},
		)
	}
}

// TestCancel_StopsALongWalk proves Copy and IsData look at the context: with a
// context that is done, a flat list of many scalars is refused with the
// context's error, not walked to the end.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCancel_StopsALongWalk(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(context.Background())
		items       = make(
			[]starlark.Value,
			4*common.POLL,
		)
	)

	for index := range items {
		items[index] = starlark.MakeInt(index)
	}

	long := starlark.NewList(items)

	cancel()

	_, err := deep.Copy(
		ctx,
		long,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"Copy = %v, want context.Canceled",
			err,
		)
	}

	_, _, err = deep.IsData(
		ctx,
		long,
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"IsData = %v, want context.Canceled",
			err,
		)
	}
}

// TestCycle_IsStillDataAndStillCopies proves the depth count did not change
// what the walks do with a value that holds itself: it is data, and its copy
// holds itself.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCycle_IsStillDataAndStillCopies(t *testing.T) {
	var (
		ctx  = context.Background()
		held = _Held(starlark.MakeInt(1))
	)

	err := held.Append(held)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := deep.Copy(
		ctx,
		held,
	)
	if err != nil {
		t.Fatal(err)
	}

	list, ok := copied.(*starlark.List)
	if !ok || list.Len() != 2 || list.Index(1) != copied {
		t.Fatalf(
			"the copy does not hold itself: %v",
			copied,
		)
	}
}
