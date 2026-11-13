package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/event"
)

// TestEvent_APostedEventIsSeenWithNoTimeToWait is a wait of zero on an event
// that has already been posted.
//
// A post that landed first is still seen, and a wait afterwards does not wait
// at all. Zero seconds is that wait: there is nothing to wait for, and the
// value is already there.
//
// Revisions:
//   - 2026-09-30 21:31: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEvent_APostedEventIsSeenWithNoTimeToWait(t *testing.T) {
	got, err := _Run(
		t,
		`
def main():
    event.post("ready", "done")

    missed = 0

    for i in range(200):
        value, err = event.wait("ready", 0)

        if err != None or value != "done":
            missed = missed + 1

    return missed
`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "0" {
		t.Fatalf(
			"missed %s of 200 waits of zero on an event already posted",
			got,
		)
	}
}

// TestEvent_ARetrySurfacesASecondPostOnTheFirstAttempt records what retry does
// with a refused post. The attempt is catching, so the refusal does not stop
// the run out from under the retry. It is not an assertion, so it is not tried
// again.
//
// Revisions:
//   - 2026-09-30 22:24: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEvent_ARetrySurfacesASecondPostOnTheFirstAttempt(t *testing.T) {
	_, err := _Run(
		t,
		`
def poster():
    event.post("once", 1)
    event.post("once", 2)

def main():
    return retry(3, poster)
`,
	)
	if !errors.Is(
		err,
		event.ErrPosted,
	) {
		t.Fatalf(
			"got %v, want ErrPosted",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"attempt 1",
	) {
		t.Fatalf(
			"a second post was tried again: %v",
			err,
		)
	}
}
