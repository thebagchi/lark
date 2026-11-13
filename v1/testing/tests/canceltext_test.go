// One cancelled run has one text: each cause appears once in it.
//
// Revisions:
//   - 2026-11-05 17:35: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
)

// TestCancelText_NamesTheCauseOnce checks that a run stopped by a deadline
// says "context deadline exceeded" once, whichever builtin it was inside, and
// that the error still matches the sentinels.
//
// Revisions:
//   - 2026-11-05 16:44: initial creation (C82, B114)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCancelText_NamesTheCauseOnce(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			"sleep",
			"sleep(5000)",
		},
		{
			"loop",
			"for i in range(100000000000):\n        pass",
		},
		{
			"spawn",
			"h = spawn(lambda: sleep(5000))\n    join(h)",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				ctx, stop := context.WithTimeout(
					t.Context(),
					200*time.Millisecond,
				)
				defer stop()

				_, err := _RunCtx(
					ctx,
					t,
					"def main():\n    "+tc.body+"\n",
				)
				if err == nil {
					t.Fatal("want an error")
				}

				t.Logf(
					"text: %v",
					err,
				)

				if n := strings.Count(
					err.Error(),
					context.DeadlineExceeded.Error(),
				); n != 1 {
					t.Fatalf(
						"cause appears %d times in %q",
						n,
						err,
					)
				}

				if !errors.Is(
					err,
					context.DeadlineExceeded,
				) {
					t.Fatalf(
						"lost DeadlineExceeded: %v",
						err,
					)
				}

				if !errors.Is(
					err,
					runtime.ErrCancelled,
				) {
					t.Fatalf(
						"lost ErrCancelled: %v",
						err,
					)
				}
			},
		)
	}
}
