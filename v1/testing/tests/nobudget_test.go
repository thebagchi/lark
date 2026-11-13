// Regression probes for the run with no memory budget: a store, an event and
// a spawn take what a script gives them, and no refusal for memory exists.
//
// Revisions:
//   - 2026-11-01 23:27: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"fmt"
	"testing"
)

const (
	// BIG_VALUE is how many bytes the values below hold. It is past the 64KB
	// ceiling that the removed budget tests used, and past the 1MB one.
	//
	// Revisions:
	//   - 2026-11-01 23:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BIG_VALUE = 4 << 20

	// SPAWNED is how many threads the spawn probe starts. The removed budget
	// charged each one, and refused the first past its ceiling.
	//
	// Revisions:
	//   - 2026-11-01 23:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPAWNED = 2000
)

// TestNoBudget_TakesWhatAScriptGives runs each call that once charged the
// budget, with a value or a count far past the old test ceilings, and wants it
// to succeed.
//
// Starting condition: no run option names a memory limit, because none exists.
// Success: each script answers the string in want.
//
// Revisions:
//   - 2026-11-01 13:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNoBudget_TakesWhatAScriptGives(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "state.set of a large string",
			src: fmt.Sprintf(
				`
def main():
    state.set("big", "x" * %d)
    return len(state.get("big"))
`, BIG_VALUE,
			),
			want: fmt.Sprint(BIG_VALUE),
		},
		{
			name: "event.post of a large string",
			src: fmt.Sprintf(
				`
def main():
    event.post("big", "x" * %d)
    value, err = event.wait("big", 0)
    return len(value)
`, BIG_VALUE,
			),
			want: fmt.Sprint(BIG_VALUE),
		},
		{
			name: "many event names",
			src: `
def main():
    for i in range(20000):
        value, err = event.wait("never%d" % i, 0)
    return "waited"
`,
			want: `"waited"`,
		},
		{
			name: "many spawned threads",
			src: fmt.Sprintf(
				`
def work():
    return 1

def main():
    total = 0
    for i in range(%d):
        handle = spawn(work)
        join(handle)
        total = total + 1
    return total
`, SPAWNED,
			),
			want: fmt.Sprint(SPAWNED),
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				t.Parallel()

				got, err := _Value(
					t,
					item.src,
				)
				if err != nil {
					t.Fatalf(
						"run: %v",
						err,
					)
				}

				if got.String() != item.want {
					t.Fatalf(
						"got %s, want %s",
						got.String(),
						item.want,
					)
				}
			},
		)
	}
}
