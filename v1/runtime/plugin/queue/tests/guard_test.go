package tests

// This file tests the queue from Go: that the item popped is the object that
// was pushed (Q420), that a popped slot is let go, that a cancelled pop takes no
// item, and that a nil builtin, a nil or zero queue and a nil context refuse
// without a panic (Q65, Q206). A script cannot pass these inputs, so the tests
// call the queue directly.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q65, Q206, Q420)

import (
	"context"
	"errors"
	"maps"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"weak"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/internal/nilsweep"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/fifo"
	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Made is a queue of the capacity given, or a failed test.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131)
func _Made(
	t *testing.T,
	capacity int,
) *fifo.Queue {
	t.Helper()

	made, err := fifo.New(capacity)
	if err != nil {
		t.Fatal(err)
	}

	return made
}

// TestFifo_PopHandsOutThePushedObject proves the queue copies nothing: for a
// list, a dict, a tuple and a string, the value pop answers is the value that
// was pushed - the same pointer for a container (Q420).
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q420)
func TestFifo_PopHandsOutThePushedObject(t *testing.T) {
	var (
		list  = starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		dict  = starlark.NewDict(1)
		set   = starlark.NewSet(1)
		tuple = starlark.Tuple{list}
	)

	tests := []struct {
		name  string
		value starlark.Value
	}{
		{
			name:  "list",
			value: list,
		},
		{
			name:  "dict",
			value: dict,
		},
		{
			name:  "set",
			value: set,
		},
		{
			name:  "string",
			value: starlark.String("x"),
		},
		{
			name:  "None",
			value: starlark.None,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					held = _Made(
						t,
						2,
					)
					answer, err = held.Push(test.value)
				)

				if err != nil || answer != starlark.None {
					t.Fatalf(
						"push gave %v, %v",
						answer,
						err,
					)
				}

				got, closed, err := held.Pop(context.Background())
				if err != nil || closed {
					t.Fatalf(
						"pop gave %v, %v, %v",
						got,
						closed,
						err,
					)
				}

				// Comparing interface values compares the pointer inside a
				// *List, *Dict or *Set. A copy would be another pointer.
				if got != test.value {
					t.Fatalf(
						"pop gave %v, a different object from %v",
						got,
						test.value,
					)
				}
			},
		)
	}

	// A tuple is a slice, which Go cannot compare: the same backing array is
	// the same object.
	held := _Made(
		t,
		1,
	)

	_, _ = held.Push(tuple)

	var (
		got, _, _ = held.Pop(context.Background())
		back, ok  = got.(starlark.Tuple)
	)

	if !ok || &back[0] != &tuple[0] {
		t.Fatalf(
			"pop gave %v, not the tuple that was pushed",
			got,
		)
	}
}

// _Canceled is a context that is already done.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131)
func _Canceled() context.Context {
	ctx, stop := context.WithCancel(context.Background())

	stop()

	return ctx
}

// TestFifo_APoppedSlotIsLetGo proves the queue keeps no reference to an item
// after it is popped: a weak pointer to the item is empty after a collection.
//
// The item is pushed into a ring that wraps, so both the cleared slot and the
// slot moved down by the compaction are checked.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q420)
func TestFifo_APoppedSlotIsLetGo(t *testing.T) {
	var (
		held = _Made(
			t,
			3,
		)
		pushed = _Churned(
			t,
			held,
		)
	)

	for held.Len() > 0 {
		_, _, err := held.Pop(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	runtime.GC()
	runtime.GC()

	for index, each := range pushed {
		if each.Value() != nil {
			t.Fatalf(
				"item %d is still reachable after it was popped",
				index,
			)
		}
	}

	// The queue must outlive the check: a queue the collector has freed holds
	// no reference to anything, and the test would pass for a queue that leaks.
	runtime.KeepAlive(held)
}

// _Churned pushes ten lists through held, popping all but the last as it goes,
// and answers a weak pointer to each. It keeps no reference to a list itself.
//
// One item is always left in the queue, so the slice never starts over by itself
// and the compaction has to move the items down.
//
// Revisions:
//   - 2026-11-13 01:35: initial creation (C131, Q420)
func _Churned(
	t *testing.T,
	held *fifo.Queue,
) []weak.Pointer[starlark.List] {
	t.Helper()

	var seen []weak.Pointer[starlark.List]

	for index := range 10 {
		item := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		seen = append(
			seen,
			weak.Make(item),
		)

		_, err := held.Push(item)
		if err != nil {
			t.Fatal(err)
		}

		if index == 0 {
			continue
		}

		_, _, err = held.Pop(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}

	return seen
}

// TestFifo_ACancelledPopTakesNoItem proves a pop whose context is done leaves
// the item in the queue for the next pop (Q416).
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q416)
func TestFifo_ACancelledPopTakesNoItem(t *testing.T) {
	held := _Made(
		t,
		2,
	)

	_, _ = held.Push(starlark.MakeInt(7))

	_, _, err := held.Pop(_Canceled())
	if !errors.Is(
		err,
		scheduler.ErrCancelled,
	) {
		t.Fatalf(
			"got %v, want ErrCancelled",
			err,
		)
	}

	if held.Len() != 1 {
		t.Fatalf(
			"the cancelled pop took the item: %d held",
			held.Len(),
		)
	}

	got, _, err := held.Pop(context.Background())
	if err != nil || got != starlark.MakeInt(7) {
		t.Fatalf(
			"the next pop gave %v, %v",
			got,
			err,
		)
	}
}

// TestFifo_CancelWakesAWaitingPop proves a context that ends while a pop waits
// ends the pop with ErrCancelled (Q419).
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q419)
func TestFifo_CancelWakesAWaitingPop(t *testing.T) {
	held := _Made(
		t,
		2,
	)

	var (
		ctx, stop = context.WithCancel(context.Background())
		answered  = make(
			chan error,
			1,
		)
	)

	go func() {
		_, _, err := held.Pop(ctx)
		answered <- err
	}()

	time.Sleep(20 * time.Millisecond)
	stop()

	select {
	case err := <-answered:
		if !errors.Is(
			err,
			scheduler.ErrCancelled,
		) {
			t.Fatalf(
				"got %v, want ErrCancelled",
				err,
			)
		}

	case <-time.After(DEADLINE):
		t.Fatal("a cancelled pop did not wake")
	}
}

// TestFifo_Capacity proves the capacity rules: 0 and a zero queue hold the
// default, a given number holds that many, and a negative one is refused with
// unpack.ErrRange (Q411, Q418).
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q411, Q418)
func TestFifo_Capacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		zero     bool
		want     int
		err      error
	}{
		{
			name:     "zero is the default",
			capacity: 0,
			want:     spec.CAPACITY,
		},
		{
			name: "a zero Queue is the default",
			zero: true,
			want: spec.CAPACITY,
		},
		{
			name:     "one",
			capacity: 1,
			want:     1,
		},
		{
			name:     "a given number",
			capacity: 1000,
			want:     1000,
		},
		{
			name:     "negative",
			capacity: -1,
			err:      unpack.ErrRange,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				built := new(fifo.Queue)

				var err error

				if !test.zero {
					built, err = fifo.New(test.capacity)
				}

				if !errors.Is(
					err,
					test.err,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.err,
					)
				}

				if err != nil {
					return
				}

				if built.Cap() != test.want {
					t.Fatalf(
						"capacity %d, want %d",
						built.Cap(),
						test.want,
					)
				}

				taken := 0

				for range test.want + 1 {
					answer, err := built.Push(starlark.MakeInt(1))
					if err != nil {
						t.Fatal(err)
					}

					if answer == starlark.String(spec.FULL) {
						break
					}

					taken++
				}

				if taken != test.want {
					t.Fatalf(
						"held %d, want %d",
						taken,
						test.want,
					)
				}
			},
		)
	}
}

// TestFifo_Guards proves a nil queue, a nil context and a nil or unbound method
// refuse without a panic (Q65, Q206).
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q65, Q206)
func TestFifo_Guards(t *testing.T) {
	var nilQueue *fifo.Queue

	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "push to a nil queue",
			call: func() error {
				_, err := nilQueue.Push(starlark.None)

				return err
			},
			want: scheduler.ErrRun,
		},
		{
			name: "pop from a nil queue",
			call: func() error {
				_, _, err := nilQueue.Pop(context.Background())

				return err
			},
			want: scheduler.ErrRun,
		},
		{
			name: "close a nil queue",
			call: nilQueue.Close,
			want: scheduler.ErrRun,
		},
		{
			name: "pop with a nil context",
			call: func() error {
				_, _, err := new(fifo.Queue).Pop(nil) //nolint:staticcheck

				return err
			},
			want: scheduler.ErrRun,
		},
		{
			name: "push on a zero queue",
			call: func() error {
				_, err := new(fifo.Queue).Push(starlark.None)

				return err
			},
		},
		{
			name: "close a zero queue twice",
			call: func() error {
				zero := new(fifo.Queue)

				_ = zero.Close()

				return zero.Close()
			},
		},
		{
			name: "push to a closed queue",
			call: func() error {
				zero := new(fifo.Queue)

				_ = zero.Close()

				_, err := zero.Push(starlark.None)

				return err
			},
			want: fifo.ErrClosed,
		},
		{
			name: "a pop method with no receiver and no run",
			call: func() error {
				_, err := fifo.QUEUE_METHODS[spec.POP].CallInternal(
					&starlark.Thread{
						Name: "t",
					},
					nil,
					nil,
				)

				return err
			},
			want: scheduler.ErrRun,
		},
		{
			name: "a push method with no receiver",
			call: func() error {
				_, err := fifo.QUEUE_METHODS[spec.PUSH].CallInternal(
					&starlark.Thread{
						Name: "t",
					},
					starlark.Tuple{starlark.None},
					nil,
				)

				return err
			},
			want: scheduler.ErrRun,
		},
		{
			name: "a close method with no receiver",
			call: func() error {
				_, err := fifo.QUEUE_METHODS[spec.CLOSE].CallInternal(
					&starlark.Thread{
						Name: "t",
					},
					nil,
					nil,
				)

				return err
			},
			want: scheduler.ErrRun,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.call()
				if test.want == nil {
					if err != nil {
						t.Fatalf(
							"got %v, want no error",
							err,
						)
					}

					return
				}

				if !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}
			},
		)
	}
}

// TestFifo_ReadsOfANilQueue proves the reads of a nil queue answer something
// and do not panic.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q65, Q206)
func TestFifo_ReadsOfANilQueue(t *testing.T) {
	var nilQueue *fifo.Queue

	if nilQueue.Len() != 0 || nilQueue.Cap() != 0 {
		t.Fatal("a nil queue holds something")
	}

	if nilQueue.Waiting() != 0 || nilQueue.Slots() != nil || nilQueue.Wakes() != 0 {
		t.Fatal("a nil queue has waiters or slots")
	}

	if nilQueue.String() != "<queue 0/0>" {
		t.Fatalf(
			"a nil queue prints %q",
			nilQueue.String(),
		)
	}

	if nilQueue.Type() != spec.TYPE || !strings.Contains(
		nilQueue.String(),
		spec.TYPE,
	) {
		t.Fatalf(
			"a nil queue is %q of type %q",
			nilQueue.String(),
			nilQueue.Type(),
		)
	}

	nilQueue.Freeze()

	if nilQueue.Truth() != starlark.True {
		t.Fatal("a queue is not true")
	}

	_, err := nilQueue.Hash()
	if err == nil {
		t.Fatal("a queue hashed")
	}

	if !slices.Equal(
		nilQueue.AttrNames(),
		[]string{"close", "pop", "push"},
	) {
		t.Fatalf(
			"methods %v",
			nilQueue.AttrNames(),
		)
	}

	missing, err := nilQueue.Attr("size")
	if missing != nil || err != nil {
		t.Fatalf(
			"a method that is not there gave %v, %v",
			missing,
			err,
		)
	}
}

// TestNilSweep_queue calls the plugin's methods on a nil and a zero receiver,
// every builtin of its module, and every method of a queue with each nil or odd
// argument shape a host or a plugin author could pass (C87, Q65, Q206). A panic
// or a hang fails the row.
//
// Revisions:
//   - 2026-11-13 01:31: initial creation (C131, Q65, Q206)
func TestNilSweep_queue(t *testing.T) {
	t.Run(
		"nil and zero receivers",
		func(t *testing.T) {
			nilsweep.Methods(
				t,
				spec.NAME,
			)
		},
	)

	members := nilsweep.Members(
		t,
		spec.NAME,
	)

	for _, name := range slices.Sorted(maps.Keys(members)) {
		t.Run(
			name,
			func(t *testing.T) {
				nilsweep.Run(
					t,
					members[name],
				)
			},
		)
	}

	for _, name := range slices.Sorted(maps.Keys(fifo.QUEUE_METHODS)) {
		t.Run(
			"method "+name,
			func(t *testing.T) {
				nilsweep.Run(
					t,
					fifo.QUEUE_METHODS[name].BindReceiver(_Made(
						t,
						4,
					)),
				)
			},
		)
	}
}
