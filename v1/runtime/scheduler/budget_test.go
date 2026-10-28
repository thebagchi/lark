package scheduler_test

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// CEILING is what the budget under test may give out.
	CEILING = 1000

	// RACERS is how many goroutines charge at once, and EACH is what each of
	// them asks for. The product is the ceiling exactly, so every charge must
	// succeed and the total must land on the ceiling - a lost update shows up
	// as room left over.
	RACERS = 100
	EACH   = 10
)

// TestBudget_RefusesWithoutReserving is the guarantee a caller leans on: a
// refused charge costs nothing, so a caller may try a smaller one.
//
// Revisions:
//   - 2026-09-24 23:26: initial creation
func TestBudget_RefusesWithoutReserving(t *testing.T) {
	var (
		budget = scheduler.NewBudget(CEILING)
		err    = budget.Charge(CEILING + 1)
	)

	if !errors.Is(
		err,
		scheduler.ErrMemory,
	) {
		t.Fatalf(
			"got %v, want ErrMemory",
			err,
		)
	}

	if budget.Held() != 0 {
		t.Fatalf(
			"a refused charge reserved %d bytes",
			budget.Held(),
		)
	}

	err = budget.Charge(CEILING)
	if err != nil {
		t.Fatalf(
			"the whole ceiling was refused: %v",
			err,
		)
	}

	if budget.Left() != 0 {
		t.Fatalf(
			"%d left after spending all of it",
			budget.Left(),
		)
	}
}

// TestBudget_CreditsBack is what makes a loop of reads possible: the second
// read spends what the first gave back.
//
// Revisions:
//   - 2026-09-24 23:26: initial creation
func TestBudget_CreditsBack(t *testing.T) {
	budget := scheduler.NewBudget(CEILING)

	for range 10 {
		err := budget.Charge(CEILING)
		if err != nil {
			t.Fatalf(
				"a charge the last one paid for was refused: %v",
				err,
			)
		}

		budget.Credit(CEILING)
	}

	if budget.Held() != 0 {
		t.Fatalf(
			"%d bytes left reserved after giving all of it back",
			budget.Held(),
		)
	}
}

// TestBudget_IsOneCeilingForEveryThread is why the count is atomic: the
// threads of a run share the memory of the process they are in, so two of them
// charging at once must not both be told there is room for one.
//
// Revisions:
//   - 2026-09-24 23:26: initial creation
func TestBudget_IsOneCeilingForEveryThread(t *testing.T) {
	budget := scheduler.NewBudget(RACERS * EACH)

	var (
		group  sync.WaitGroup
		mutex  sync.Mutex
		failed []error
	)

	for range RACERS {
		group.Add(1)

		go func() {
			defer group.Done()

			err := budget.Charge(EACH)
			if err == nil {
				return
			}

			mutex.Lock()

			failed = append(
				failed,
				err,
			)
			mutex.Unlock()
		}()
	}

	group.Wait()

	if len(failed) > 0 {
		t.Fatalf(
			"%d of %d charges were refused inside the ceiling: %v",
			len(failed),
			RACERS,
			failed[0],
		)
	}

	if budget.Left() != 0 {
		t.Fatalf(
			"%d bytes left after charging the ceiling exactly, so a charge was lost",
			budget.Left(),
		)
	}
}

// TestRegression_Budget rejects a charge that wraps held+size past int64.
//
// Before this fix, Charge(math.MaxInt64) after a held of 1 wrapped to a
// negative sum, passed the ceiling check, and left held at math.MinInt64.
//
// Revisions:
//   - 2026-10-24 16:00: initial creation
func TestRegression_Budget(t *testing.T) {
	budget := scheduler.NewBudget(scheduler.CEILING)

	if err := budget.Charge(1); err != nil {
		t.Fatal(err)
	}

	err := budget.Charge(math.MaxInt64)
	if !errors.Is(
		err,
		scheduler.ErrMemory,
	) {
		t.Fatalf(
			"held %d, err %v",
			budget.Held(),
			err,
		)
	}

	if budget.Held() != 1 {
		t.Fatalf(
			"held %d after a refused charge, want 1",
			budget.Held(),
		)
	}
}

// TestBudget_IgnoresNothing keeps a zero or negative size from moving the
// count, since a caller computing a size from a stat can hand over either.
//
// Revisions:
//   - 2026-09-24 23:26: initial creation
func TestBudget_IgnoresNothing(t *testing.T) {
	var (
		budget = scheduler.NewBudget(CEILING)
		err    = budget.Charge(0)
	)
	if err != nil {
		t.Fatalf(
			"charging nothing was refused: %v",
			err,
		)
	}

	budget.Credit(0)
	budget.Credit(-1)

	if budget.Held() != 0 {
		t.Fatalf(
			"held %d after charging and crediting nothing",
			budget.Held(),
		)
	}
}
