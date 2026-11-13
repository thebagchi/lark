package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

// TestNilSweep_memwatch calls every exported function of the package with each
// nil or zero argument a host or a plugin author could pass (C87, Q2, Q85).
// A row passes when the call returns before the timeout and, where want names
// a sentinel, the error is that sentinel. A panic or a hang fails the row.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestNilSweep_memwatch(t *testing.T) {
	var (
		nilCtx context.Context
		// A watch that trips at the first read, for the nil trip row.
		trips = &common.MemoryWatch{
			Interval: time.Millisecond,
			Source: func() (*common.MemoryReading, error) {
				return &common.MemoryReading{
					Used:  100,
					Total: 100,
				}, nil
			},
		}
	)

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "Start nil context, nil watch",
			call: func() error {
				memwatch.Start(
					nilCtx,
					nil,
					nil,
				)()

				return nil
			},
		},
		{
			name: "Start nil context",
			call: func() error {
				memwatch.Start(
					nilCtx,
					new(common.MemoryWatch),
					func(error) {
						// A trip is no concern of this row.
					},
				)()

				return nil
			},
		},
		{
			name: "Start nil trip function",
			call: func() error {
				stop := memwatch.Start(
					t.Context(),
					trips,
					nil,
				)

				time.Sleep(50 * time.Millisecond)
				stop()

				return nil
			},
		},
		{
			name: "Start nil trip function and nil source",
			call: func() error {
				memwatch.Start(
					t.Context(),
					new(common.MemoryWatch),
					nil,
				)()

				return nil
			},
		},
		{
			name: "Start stop twice",
			call: func() error {
				stop := memwatch.Start(
					t.Context(),
					new(common.MemoryWatch),
					nil,
				)

				stop()
				stop()

				return nil
			},
		},
		{
			name: "Fit nil reading function",
			call: func() error {
				return memwatch.Fit(
					"who",
					common.ROOM_CEILING*2,
					nil,
				)
			},
			want: memwatch.ErrRoom,
		},
		{
			name: "Fit nil reading function, size at the skip limit",
			call: func() error {
				return memwatch.Fit(
					"who",
					common.ROOM_SKIP,
					nil,
				)
			},
		},
		{
			name: "Fit nil reading function, size past skip but under ceiling",
			call: func() error {
				return memwatch.Fit(
					"who",
					common.ROOM_SKIP+1,
					nil,
				)
			},
		},
		{
			name: "Fit reading function answers nil, nil",
			call: func() error {
				return memwatch.Fit(
					"who",
					common.ROOM_CEILING*2,
					func() (*common.MemoryReading, error) {
						return nil, nil
					},
				)
			},
			want: memwatch.ErrRoom,
		},
		{
			name: "Fit empty name, zero size",
			call: func() error {
				return memwatch.Fit(
					"",
					0,
					nil,
				)
			},
		},
		{
			name: "Room empty name, zero size",
			call: func() error {
				return memwatch.Room(
					"",
					0,
				)
			},
		},
		{
			name: "Check",
			call: func() error {
				_ = memwatch.Check()

				return nil
			},
		},
		{
			name: "System reading",
			call: func() error {
				_, _ = memwatch.System()()

				return nil
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
