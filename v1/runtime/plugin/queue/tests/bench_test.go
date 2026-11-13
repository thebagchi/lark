package tests

// Benchmarks of the queue at the capacities where the cost of a push and a pop
// showed: 64, the default; 18,432 and 1,109,504, where the backing slice of
// an earlier version had no spare room (C131, round 2).
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131)

import (
	"context"
	"strconv"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/queue/internal/fifo"
)

// CAPACITIES are the capacities the benchmarks run at.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131)
var (
	CAPACITIES = []int{64, 18432, 1109504}
)

// BenchmarkFifo_Steady is one pop and one push on a queue that is full: the
// steady state of a producer and a consumer that keep up with each other. Its
// cost must not depend on the capacity.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131)
func BenchmarkFifo_Steady(b *testing.B) {
	for _, capacity := range CAPACITIES {
		b.Run(
			strconv.Itoa(capacity),
			func(b *testing.B) {
				held, err := fifo.New(capacity)
				if err != nil {
					b.Fatal(err)
				}

				for range capacity {
					_, _ = held.Push(starlark.None)
				}

				ctx := context.Background()

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, _, _ = held.Pop(ctx)
					_, _ = held.Push(starlark.None)
				}
			},
		)
	}
}

// BenchmarkFifo_Shallow is one push and one pop on a queue that is nearly
// empty: the cost of a hand-over when the consumer is ahead.
//
// Revisions:
//   - 2026-11-13 09:40: initial creation (C131)
func BenchmarkFifo_Shallow(b *testing.B) {
	for _, capacity := range CAPACITIES {
		b.Run(
			strconv.Itoa(capacity),
			func(b *testing.B) {
				held, err := fifo.New(capacity)
				if err != nil {
					b.Fatal(err)
				}

				ctx := context.Background()

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, _ = held.Push(starlark.None)
					_, _, _ = held.Pop(ctx)
				}
			},
		)
	}
}
