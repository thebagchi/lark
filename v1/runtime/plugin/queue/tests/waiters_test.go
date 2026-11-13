package tests

// This file tests the list of waiting pops (Q428): that the pop which has
// waited longest gets the next item, that a pop which arrives later does not
// pass those that wait, that a pop cancelled anywhere in the list leaves it and
// swallows no item, that a close wakes all and still serves the items in order,
// and that no item is lost, repeated or reordered while pushes, pops, cancels
// and a close race.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/fifo"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// THOUSAND is the step between the numbers of two pushers: pusher 2 pushes 2000,
// 2001 and so on, so a number tells who pushed it.
//
// Revisions:
//   - 2026-11-13 09:44: initial creation (C131, Q428)
const (
	THOUSAND = 1000
)

// _Popped is what one Pop answered.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
type _Popped struct {
	value  starlark.Value
	closed bool
	err    error
}

// _Popping starts a Pop on held in a goroutine and answers the channel its
// answer arrives on.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func _Popping(
	ctx context.Context,
	held *fifo.Queue,
) <-chan *_Popped {
	answered := make(
		chan *_Popped,
		1,
	)

	go func() {
		value, closed, err := held.Pop(ctx)
		answered <- &_Popped{
			value:  value,
			closed: closed,
			err:    err,
		}
	}()

	return answered
}

// _Is reports whether value is the int n.
//
// Revisions:
//   - 2026-11-13 09:43: initial creation (C131, Q428)
func _Is(
	value starlark.Value,
	n int,
) bool {
	return value != nil && value.String() == starlark.MakeInt(n).String()
}

// _Number is the item a pusher puts: its own number, then its position.
//
// Revisions:
//   - 2026-11-13 09:44: initial creation (C131, Q428)
func _Number(
	who int,
	index int,
) int {
	return who*THOUSAND + index
}

// _Standing is a condition that holds when n pops stand in the list of held.
//
// Revisions:
//   - 2026-11-13 09:45: initial creation (C131, Q428)
func _Standing(
	held *fifo.Queue,
	n int,
) func() bool {
	return func() bool {
		return held.Waiting() == n
	}
}

// _Until waits until condition holds, or fails the test at the deadline. It
// waits on a state of the queue, not on a clock.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func _Until(
	t *testing.T,
	what string,
	condition func() bool,
) {
	t.Helper()

	deadline := time.Now().Add(DEADLINE)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf(
				"gave up waiting until %s",
				what,
			)
		}

		time.Sleep(50 * time.Microsecond)
	}
}

// _Within waits for an answer, or fails the test at the deadline.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func _Within(
	t *testing.T,
	answered <-chan *_Popped,
) *_Popped {
	t.Helper()

	select {
	case got := <-answered:
		return got

	case <-time.After(DEADLINE):
		t.Fatal("a pop did not answer")
	}

	return nil
}

// _Waits starts n pops on held, one at a time, each only after the one before
// it stands in the list, so the order of waiting is the order of the slice.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func _Waits(
	t *testing.T,
	held *fifo.Queue,
	n int,
) ([]<-chan *_Popped, []context.CancelFunc) {
	t.Helper()

	var (
		answers []<-chan *_Popped
		cancels []context.CancelFunc
	)

	for index := range n {
		ctx, stop := context.WithCancel(context.Background())

		answers = append(
			answers,
			_Popping(
				ctx,
				held,
			),
		)
		cancels = append(
			cancels,
			stop,
		)

		_Until(
			t,
			"the pop stands in the list",
			_Standing(
				held,
				index+1,
			),
		)
	}

	return answers, cancels
}

// TestFifo_TheLongestWaiterGetsTheNextItem starts five pops in a known order,
// pushes five items, and requires the nth pop to get the nth item.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func TestFifo_TheLongestWaiterGetsTheNextItem(t *testing.T) {
	held := _Made(
		t,
		8,
	)

	var (
		answers, cancels = _Waits(
			t,
			held,
			5,
		)
	)

	defer func() {
		for _, stop := range cancels {
			stop()
		}
	}()

	for index := range 5 {
		_, _ = held.Push(starlark.MakeInt(index))
	}

	for index, answered := range answers {
		got := _Within(
			t,
			answered,
		)
		if got.err != nil || !_Is(
			got.value,
			index,
		) {
			t.Fatalf(
				"waiter %d got %v, %v",
				index,
				got.value,
				got.err,
			)
		}
	}

	if held.Waiting() != 0 || held.Len() != 0 {
		t.Fatalf(
			"%d wait and %d items are held after all were served",
			held.Waiting(),
			held.Len(),
		)
	}
}

// TestFifo_APushWakesTheFirstWaiterOnly has five pops wait, pushes one item and
// counts the wakes the queue made. A push wakes the first waiter and no other,
// so there is one wake, however the scheduler orders the goroutines: the pop
// that took the item leaves an empty ring and wakes nobody. A close then wakes
// each of the four that are left, once.
//
// Revisions:
//   - 2026-11-13 11:25: initial creation (C131, Q428, Q314)
func TestFifo_APushWakesTheFirstWaiterOnly(t *testing.T) {
	held := _Made(
		t,
		8,
	)

	var (
		answers, cancels = _Waits(
			t,
			held,
			5,
		)
	)

	defer func() {
		for _, stop := range cancels {
			stop()
		}
	}()

	if held.Wakes() != 0 {
		t.Fatalf(
			"%d wakes before any push",
			held.Wakes(),
		)
	}

	_, _ = held.Push(starlark.MakeInt(0))

	got := _Within(
		t,
		answers[0],
	)
	if got.err != nil || !_Is(
		got.value,
		0,
	) {
		t.Fatalf(
			"the first waiter got %v, %v",
			got.value,
			got.err,
		)
	}

	_Until(
		t,
		"the taker has left the list",
		_Standing(
			held,
			4,
		),
	)

	if held.Wakes() != 1 {
		t.Fatalf(
			"%d wakes for one push with five waiting, want 1",
			held.Wakes(),
		)
	}

	_ = held.Close()

	for _, answered := range answers[1:] {
		got := _Within(
			t,
			answered,
		)
		if !got.closed {
			t.Fatalf(
				"a waiter got %v, %v after a close",
				got.value,
				got.err,
			)
		}
	}
}

// TestFifo_APopThatArrivesLaterDoesNotPassTheWaiters pushes an item for a
// waiting pop, and then starts a second pop at once. The first pop must get the
// item, and the second must wait for the next, whichever of them the scheduler
// runs first. It runs 100 times, because the order of two goroutines is the
// thing under test.
//
// Revisions:
//   - 2026-11-13 09:48: initial creation (C131, Q428)
func TestFifo_APopThatArrivesLaterDoesNotPassTheWaiters(t *testing.T) {
	for round := range 100 {
		held := _Made(
			t,
			8,
		)

		var (
			answers, cancels = _Waits(
				t,
				held,
				1,
			)
			ctx, stop = context.WithCancel(context.Background())
		)

		_, _ = held.Push(starlark.String("first"))

		var (
			second = _Popping(
				ctx,
				held,
			)
			got = _Within(
				t,
				answers[0],
			)
		)
		if got.err != nil || got.value.String() != `"first"` {
			t.Fatalf(
				"round %d: the waiter got %v, %v",
				round,
				got.value,
				got.err,
			)
		}

		_Until(
			t,
			"the second pop stands in the list",
			_Standing(
				held,
				1,
			),
		)

		_, _ = held.Push(starlark.String("second"))

		got = _Within(
			t,
			second,
		)
		if got.err != nil || got.value.String() != `"second"` {
			t.Fatalf(
				"round %d: the later pop got %v, %v",
				round,
				got.value,
				got.err,
			)
		}

		stop()
		cancels[0]()
	}
}

// TestFifo_AWokenFirstWaiterThatIsCancelledPassesTheItemOn has two pops wait, and
// then pushes one item and cancels the first pop at once. The first pop either
// took the item before it was cancelled, or was cancelled with the item still
// in the queue. In the second case the second pop must get the item: the first
// pop was woken for it and must wake the next as it leaves. It runs 200 times,
// because which of the two happens is up to the scheduler.
//
// Revisions:
//   - 2026-11-13 09:48: initial creation (C131, Q428)
func TestFifo_AWokenFirstWaiterThatIsCancelledPassesTheItemOn(t *testing.T) {
	for round := range 200 {
		held := _Made(
			t,
			8,
		)

		var (
			answers, cancels = _Waits(
				t,
				held,
				2,
			)
		)

		_, _ = held.Push(starlark.MakeInt(round))

		cancels[0]()

		first := _Within(
			t,
			answers[0],
		)

		if first.err == nil {
			if !_Is(
				first.value,
				round,
			) {
				t.Fatalf(
					"round %d: the first pop got %v",
					round,
					first.value,
				)
			}

			cancels[1]()

			continue
		}

		second := _Within(
			t,
			answers[1],
		)
		if second.err != nil || !_Is(
			second.value,
			round,
		) {
			t.Fatalf(
				"round %d: the item was lost: the second pop got %v, %v",
				round,
				second.value,
				second.err,
			)
		}
	}
}

// TestFifo_ACancelAnywhereInTheListLosesNoItem cancels the first, a middle and
// the last of four waiting pops in turn. The cancelled pop answers ErrCancelled
// and leaves the list, and three pushes then go to the other three in order.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func TestFifo_ACancelAnywhereInTheListLosesNoItem(t *testing.T) {
	tests := []struct {
		name   string
		cancel int
	}{
		{
			"the first",
			0,
		},
		{
			"a middle one",
			1,
		},
		{
			"another middle one",
			2,
		},
		{
			"the last",
			3,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				held := _Made(
					t,
					8,
				)

				var (
					answers, cancels = _Waits(
						t,
						held,
						4,
					)
				)

				cancels[test.cancel]()

				got := _Within(
					t,
					answers[test.cancel],
				)
				if !errors.Is(
					got.err,
					scheduler.ErrCancelled,
				) || got.value != nil {
					t.Fatalf(
						"the cancelled pop gave %v, %v",
						got.value,
						got.err,
					)
				}

				_Until(
					t,
					"the cancelled pop left the list",
					_Standing(
						held,
						3,
					),
				)

				for index := range 3 {
					_, _ = held.Push(starlark.MakeInt(index))
				}

				next := 0

				for index, answered := range answers {
					if index == test.cancel {
						continue
					}

					got := _Within(
						t,
						answered,
					)
					if got.err != nil || !_Is(
						got.value,
						next,
					) {
						t.Fatalf(
							"waiter %d got %v, %v, want %d",
							index,
							got.value,
							got.err,
							next,
						)
					}

					next++
				}

				for _, stop := range cancels {
					stop()
				}
			},
		)
	}
}

// TestFifo_ACloseServesTheItemsInOrderAndWakesTheRest starts five pops, pushes
// two items and closes. The first two pops get the two items in order, and the
// other three answer closed.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func TestFifo_ACloseServesTheItemsInOrderAndWakesTheRest(t *testing.T) {
	held := _Made(
		t,
		8,
	)

	var (
		answers, cancels = _Waits(
			t,
			held,
			5,
		)
	)

	defer func() {
		for _, stop := range cancels {
			stop()
		}
	}()

	_, _ = held.Push(starlark.MakeInt(0))
	_, _ = held.Push(starlark.MakeInt(1))
	_ = held.Close()

	for index, answered := range answers {
		got := _Within(
			t,
			answered,
		)

		if index < 2 {
			if got.err != nil || got.closed || !_Is(
				got.value,
				index,
			) {
				t.Fatalf(
					"waiter %d got %v, closed %v, %v",
					index,
					got.value,
					got.closed,
					got.err,
				)
			}

			continue
		}

		if got.err != nil || !got.closed || got.value != nil {
			t.Fatalf(
				"waiter %d got %v, closed %v, %v, want closed",
				index,
				got.value,
				got.closed,
				got.err,
			)
		}
	}

	if held.Waiting() != 0 {
		t.Fatalf(
			"%d pops still wait after a close",
			held.Waiting(),
		)
	}
}

// TestFifo_NothingIsLostWhilePushesPopsCancelsAndACloseRace runs four pushers
// and six poppers, whose pops are often cancelled by a short deadline, and then
// closes. Every item that a push accepted comes out exactly once, none comes
// out twice, each pusher's items come out in the order they were pushed within
// one popper, and no pop is left in the list.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131, Q428)
func TestFifo_NothingIsLostWhilePushesPopsCancelsAndACloseRace(t *testing.T) {
	const (
		PUSHERS = 4
		POPPERS = 6
		EACH    = 300
	)

	var (
		held = _Made(
			t,
			8,
		)
		pushers sync.WaitGroup
		poppers sync.WaitGroup
		guard   sync.Mutex
		popped  [][]int
	)

	for range POPPERS {
		poppers.Add(1)

		go func() {
			defer poppers.Done()

			var mine []int

			for {
				ctx, stop := context.WithTimeout(
					context.Background(),
					150*time.Microsecond,
				)

				var (
					value, closed, err = held.Pop(ctx)
				)

				stop()

				if closed {
					break
				}

				if err != nil {
					continue
				}

				number, _ := starlark.AsInt32(value.(starlark.Int))

				mine = append(
					mine,
					int(number),
				)
			}

			guard.Lock()
			defer guard.Unlock()

			popped = append(
				popped,
				mine,
			)
		}()
	}

	for who := range PUSHERS {
		pushers.Add(1)

		go func() {
			defer pushers.Done()

			for index := range EACH {
				for {
					answer, err := held.Push(starlark.MakeInt(_Number(
						who,
						index,
					)))
					if err != nil {
						t.Error(err)

						return
					}

					if answer == starlark.None {
						break
					}

					time.Sleep(10 * time.Microsecond)
				}
			}
		}()
	}

	pushers.Wait()

	_ = held.Close()

	poppers.Wait()

	var every []int

	for _, mine := range popped {
		every = append(
			every,
			mine...,
		)

		for who := range PUSHERS {
			var own []int

			for _, number := range mine {
				if number/THOUSAND == who {
					own = append(
						own,
						number,
					)
				}
			}

			if !slices.IsSorted(own) {
				t.Fatalf(
					"pusher %d came out of order in one popper: %v",
					who,
					own,
				)
			}
		}
	}

	slices.Sort(every)

	if len(every) != PUSHERS*EACH {
		t.Fatalf(
			"%d items came out, want %d",
			len(every),
			PUSHERS*EACH,
		)
	}

	for index, number := range every {
		want := (index/EACH)*THOUSAND + index%EACH
		if number != want {
			t.Fatalf(
				"item %d is %d, want %d: one was lost or came out twice",
				index,
				number,
				want,
			)
		}
	}

	if held.Waiting() != 0 || held.Len() != 0 {
		t.Fatalf(
			"%d pops wait and %d items are held at the end",
			held.Waiting(),
			held.Len(),
		)
	}
}
