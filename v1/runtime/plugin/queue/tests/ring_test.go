package tests

// This file tests the ring the items sit in, and the state of a queue as a
// print shows it: that a slot which holds no item is nil after every push, pop,
// wrap and growth, that the order survives a wrap and a growth, that the ring
// never outgrows the capacity, and that the state is read under the lock
// (C131, Q420, Q427).
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q420, Q427)

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/fifo"
)

// _Held is how many slots of the ring hold an item.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q420)
func _Held(held *fifo.Queue) int {
	taken := 0

	for _, slot := range held.Slots() {
		if slot != nil {
			taken++
		}
	}

	return taken
}

// _Course is a queue and the model it is checked against: the numbers it should
// hold, front first, and the next number to push.
//
// Revisions:
//   - 2026-11-13 09:44: initial creation (C131, Q420)
type _Course struct {
	held  *fifo.Queue
	model []int
	next  int
}

// Push pushes the next number. A push the queue accepts goes into the model.
//
// Revisions:
//   - 2026-11-13 09:44: initial creation (C131, Q420)
func (c *_Course) Push() error {
	answer, err := c.held.Push(starlark.MakeInt(c.next))
	if err != nil {
		return err
	}

	if answer == starlark.None {
		c.model = append(
			c.model,
			c.next,
		)
		c.next++
	}

	return nil
}

// Pop pops when the model holds a number, and requires the answer to be the one
// at the front of it.
//
// Revisions:
//   - 2026-11-13 09:44: initial creation (C131, Q420)
func (c *_Course) Pop() error {
	if len(c.model) == 0 {
		return nil
	}

	got, _, err := c.held.Pop(context.Background())
	if err != nil {
		return err
	}

	if !_Is(
		got,
		c.model[0],
	) {
		return fmt.Errorf(
			"popped %v, want %d",
			got,
			c.model[0],
		)
	}

	c.model = c.model[1:]

	return nil
}

// TestFifo_EverySlotWithoutAnItemIsNil runs a long, seeded run of pushes and
// pops on queues of several capacities, and after each step requires: the slots
// that hold an item are as many as the queue says it holds, the items come out
// in the order they went in, and the ring is no longer than the capacity. A slot
// that kept a popped item, or kept a copy left by a growth or a wrap, would hold
// one more item than the queue does.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q420)
func TestFifo_EverySlotWithoutAnItemIsNil(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		pushing  float64
	}{
		{
			"one",
			1,
			0.5,
		},
		{
			"three, filling",
			3,
			0.7,
		},
		{
			"seven, below the first ring",
			7,
			0.55,
		},
		{
			"eight, the first ring",
			8,
			0.6,
		},
		{
			"nine, one past the first ring",
			9,
			0.6,
		},
		{
			"sixty-four, mostly full",
			64,
			0.52,
		},
		{
			"a hundred, mostly empty",
			100,
			0.45,
		},
		{
			"a thousand and one, growing",
			1001,
			0.6,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				var (
					course = &_Course{
						held: _Made(
							t,
							test.capacity,
						),
					}
					dice = rand.New(rand.NewPCG(
						7,
						uint64(test.capacity),
					))
				)

				for step := range 6000 {
					var err error

					if dice.Float64() < test.pushing {
						err = course.Push()
					} else {
						err = course.Pop()
					}

					if err != nil {
						t.Fatalf(
							"step %d: %v",
							step,
							err,
						)
					}

					var (
						want  = len(course.model)
						slots = _Held(course.held)
					)

					if slots != want || course.held.Len() != want {
						t.Fatalf(
							"step %d: %d slots, queue %d, want %d",
							step,
							slots,
							course.held.Len(),
							want,
						)
					}

					if len(course.held.Slots()) > test.capacity {
						t.Fatalf(
							"step %d: a ring of %d slots for %d",
							step,
							len(course.held.Slots()),
							test.capacity,
						)
					}
				}
			},
		)
	}
}

// TestFifo_AGrowthKeepsTheOrderOfAWrappedRing fills a ring of eight, pops five
// so the items wrap, pushes past eight so the ring must grow while it is
// wrapped, and requires the order and the slots to be right.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q420)
func TestFifo_AGrowthKeepsTheOrderOfAWrappedRing(t *testing.T) {
	held := _Made(
		t,
		20,
	)

	for index := range 8 {
		_, _ = held.Push(starlark.MakeInt(index))
	}

	for range 5 {
		_, _, _ = held.Pop(context.Background())
	}

	for index := 8; index < 20; index++ {
		_, _ = held.Push(starlark.MakeInt(index))
	}

	if _Held(held) != 15 || held.Len() != 15 {
		t.Fatalf(
			"%d slots hold an item and the queue holds %d, want 15",
			_Held(held),
			held.Len(),
		)
	}

	for want := 5; want < 20; want++ {
		got, _, err := held.Pop(context.Background())
		if err != nil || got.String() != starlark.MakeInt(want).String() {
			t.Fatalf(
				"popped %v, %v, want %d",
				got,
				err,
				want,
			)
		}
	}

	if _Held(held) != 0 {
		t.Fatalf(
			"%d slots still hold an item after the queue was drained",
			_Held(held),
		)
	}
}

// TestFifo_AFullQueueDoesNotGrowTheRingPastItsCapacity pushes and pops at a
// full queue for a long time and requires the ring to stay the size of the
// capacity.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q420)
func TestFifo_AFullQueueDoesNotGrowTheRingPastItsCapacity(t *testing.T) {
	for _, capacity := range []int{1, 5, 8, 64, 143, 1000} {
		held := _Made(
			t,
			capacity,
		)

		for index := range capacity {
			_, _ = held.Push(starlark.MakeInt(index))
		}

		for index := range 3 * capacity {
			_, _, _ = held.Pop(context.Background())
			_, _ = held.Push(starlark.MakeInt(index))
		}

		if len(held.Slots()) != capacity || _Held(held) != capacity {
			t.Fatalf(
				"capacity %d: %d slots, %d hold an item",
				capacity,
				len(held.Slots()),
				_Held(held),
			)
		}
	}
}

// TestFifo_StateIsReadUnderTheLock reads State, String and Len from one
// goroutine while others push, pop and close, under the race detector, and
// requires each reading to be whole: never more items than the capacity, and
// a closed queue stays closed.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q427)
func TestFifo_StateIsReadUnderTheLock(t *testing.T) {
	var (
		held = _Made(
			t,
			16,
		)
		group  sync.WaitGroup
		closed bool
	)

	group.Add(2)

	go func() {
		defer group.Done()

		for index := range 3000 {
			_, _ = held.Push(starlark.MakeInt(index))
		}

		_ = held.Close()
	}()

	go func() {
		defer group.Done()

		for {
			_, drained, err := held.Pop(context.Background())
			if err != nil || drained {
				return
			}
		}
	}()

	for range 3000 {
		count, capacity, now := held.State()
		if count < 0 || count > capacity {
			t.Fatalf(
				"state %d/%d",
				count,
				capacity,
			)
		}

		if closed && !now {
			t.Fatal("a closed queue was read as open")
		}

		closed = closed || now

		_ = held.String()
	}

	group.Wait()

	if held.String() != "<queue 0/16 closed>" {
		t.Fatalf(
			"got %q",
			held.String(),
		)
	}
}

// TestFifo_APushAndAPopAtAFullQueueDoNotCopyTheQueue does 2,000 pops and pushes
// on a full queue whose capacity is 1,109,504, the size at which an earlier
// version, which compacted its slice, copied the whole queue on every push and
// took about 3 seconds. A ring takes a few milliseconds. The bound is 100
// times what the ring takes and a third of what the copy took, so it does not
// depend on the speed of the machine.
//
// Revisions:
//   - 2026-11-13 09:47: initial creation (C131, round 2)
func TestFifo_APushAndAPopAtAFullQueueDoNotCopyTheQueue(t *testing.T) {
	const (
		CAPACITY = 1109504
		CYCLES   = 2000
		BOUND    = time.Second
	)

	held := _Made(
		t,
		CAPACITY,
	)

	for range CAPACITY {
		_, _ = held.Push(starlark.None)
	}

	started := time.Now()

	for range CYCLES {
		_, _, _ = held.Pop(context.Background())
		_, _ = held.Push(starlark.None)
	}

	if took := time.Since(started); took > BOUND {
		t.Fatalf(
			"%d cycles took %v, want under %v: the queue is copied",
			CYCLES,
			took,
			BOUND,
		)
	}
}
