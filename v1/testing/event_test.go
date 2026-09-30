package testing_test

import (
	"testing"

	_ "github.com/thebagchi/lark/v1/runtime/plugin/event"
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
func TestEvent_APostedEventIsSeenWithNoTimeToWait(t *testing.T) {
	got, err := _Run(t, `
def main():
    event.post("ready", "done")

    missed = 0

    for i in range(200):
        value, err = event.wait("ready", 0)

        if err != None or value != "done":
            missed = missed + 1

    return missed
`)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != "0" {
		t.Fatalf("missed %s of 200 waits of zero on an event already posted", got)
	}
}
