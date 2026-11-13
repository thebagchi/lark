package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestNilSweep_trail calls every exported function of the package with each
// nil or zero argument a host or a plugin author could pass (C87, Q2, Q85).
// A row passes when the call returns before the timeout and, where want names
// a sentinel, the error is that sentinel. A nil trail has no sentinel: its row
// matches the text "walk has no trail". A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNilSweep_trail(t *testing.T) {
	var (
		nilCtx context.Context
		// Slices: values a plugin author may hand to Enter although it asks for
		// a pointer or nil.
		sliceOne = []int{1}
		sliceTwo = []int{2}
	)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "New nil context",
			call: func() error {
				return trail.Tick(trail.New(nilCtx))
			},
		},
		{
			name: "Bounded nil context",
			call: func() error {
				return trail.Tick(trail.Bounded(nilCtx))
			},
		},
		{
			name: "Enter nil trail",
			call: func() error {
				return _NoTrail(trail.Enter(
					nil,
					starlark.NewList(nil),
				))
			},
		},
		{
			name: "Tick nil trail",
			call: func() error {
				return _NoTrail(trail.Tick(nil))
			},
		},
		{
			name: "Leave nil trail",
			call: func() error {
				trail.Leave(nil)

				return nil
			},
		},
		{
			name: "Enter zero trail nil container",
			call: func() error {
				return trail.Enter(
					new(common.Trail),
					nil,
				)
			},
		},
		{
			name: "Enter zero trail with cycles",
			call: func() error {
				path := &common.Trail{
					Cycles: true,
				}

				var err = trail.Enter(
					path,
					starlark.NewList(nil),
				)
				if err != nil {
					return err
				}

				trail.Leave(path)

				return nil
			},
		},
		{
			name: "Enter two slices",
			call: func() error {
				return _Twice(
					sliceOne,
					sliceTwo,
				)
			},
		},
		{
			name: "Enter two maps",
			call: func() error {
				return _Twice(
					map[string]int{},
					map[string]int{},
				)
			},
		},
		{
			name: "Enter two structs holding a slice",
			call: func() error {
				return _Twice(
					_Holder{
						sliceOne,
					},
					_Holder{
						sliceTwo,
					},
				)
			},
		},
		{
			name: "Enter two funcs",
			call: func() error {
				return _Twice(
					_Nothing,
					_Nothing,
				)
			},
		},
		{
			name: "Leave on a trail never entered",
			call: func() error {
				trail.Leave(new(common.Trail))
				trail.Leave(&common.Trail{
					Depth:  3,
					Cycles: true,
				})

				return nil
			},
		},
		{
			name: "Tick zero trail past the poll",
			call: func() error {
				path := new(common.Trail)

				for range common.POLL + 1 {
					err := trail.Tick(path)
					if err != nil {
						return err
					}
				}

				return nil
			},
		},
		{
			name: "Tick trail with a nil error function",
			call: func() error {
				path := &common.Trail{
					Count: common.POLL,
				}

				return trail.Tick(path)
			},
		},
		{
			name: "Enter a slice then the same slice",
			call: func() error {
				return _Twice(
					sliceOne,
					sliceOne,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					got      error
					panicked any
					finished = make(chan struct{})
				)

				go func() {
					defer close(finished)
					defer func() {
						panicked = recover()
					}()

					got = test.call()
				}()

				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("hung")
				}

				if panicked != nil {
					t.Fatalf(
						"panicked: %v",
						panicked,
					)
				}

				if test.want != nil && !errors.Is(
					got,
					test.want,
				) {
					t.Fatalf(
						"error %v, want %v",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// _Holder is a struct that holds a value of any type, so the type is
// comparable by the language and not by the value it holds.
//
// Revisions:
//   - 2026-11-04 19:51: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Holder struct {
	any
}

// _Twice enters two containers, one after the other, on a trail that looks for
// cycles, and returns the first error.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Twice(
	first any,
	second any,
) error {
	var (
		path = trail.New(context.Background())
		err  = trail.Enter(
			path,
			first,
		)
	)
	if err != nil {
		return err
	}

	return trail.Enter(
		path,
		second,
	)
}

// _Nothing is a function that does nothing; two of its values are two func
// values, which the language cannot compare.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Nothing() {
	// Empty on purpose.
}

// _NoTrail returns nil when err is the plain error of a nil trail, and an error
// saying so otherwise.
//
// Revisions:
//   - 2026-11-08 19:03: initial creation (C98, Q268)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _NoTrail(err error) error {
	if err == nil || !strings.Contains(
		err.Error(),
		"walk has no trail",
	) {
		return errors.New("want the walk has no trail error")
	}

	return nil
}
