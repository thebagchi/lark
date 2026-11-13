package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// TestZeroExecution proves a zero Execution answers Status and Snapshot with an
// empty graph, not nil and not a panic.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestZeroExecution(t *testing.T) {
	cases := []struct {
		name string
		ask  func(e *observe.Execution) int
	}{
		{
			name: "Status",
			ask: func(e *observe.Execution) int {
				return len(e.Status().GetFunctions())
			},
		},
		{
			name: "Snapshot",
			ask: func(e *observe.Execution) int {
				return len(e.Snapshot().GetFunctions())
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if got := tc.ask(new(observe.Execution)); got != 0 {
					t.Fatalf(
						"want no functions, got %d",
						got,
					)
				}
			},
		)
	}
}

// TestZeroExecutionWait proves Wait on a zero Execution returns an error at
// once and Done is a closed channel.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 1)
//   - 2026-11-04 23:50: fails after 5 s rather than hanging (C86 fix round 2)
//   - 2026-11-08 13:25: the error is a plain one, ErrNotStarted being dropped (C98, Q249)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestZeroExecutionWait(t *testing.T) {
	var (
		e      = new(observe.Execution)
		failed = make(
			chan error,
			1,
		)
	)

	go func() {
		_, err := e.Wait()
		failed <- err
	}()

	select {
	case err := <-failed:
		if err == nil || !strings.Contains(
			err.Error(),
			"not started",
		) {
			t.Errorf(
				"want a not-started error, got %v",
				err,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked")
	}

	select {
	case <-e.Done():
	default:
		t.Fatal("Done is not closed")
	}
}
