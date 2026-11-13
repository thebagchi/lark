package observe

import (
	"slices"
)

// _Thread is one thread of the run that is in the middle of a call: its id,
// the calls it has started and not ended, and its number at the node it is
// executing.
//
// calls holds the calls newest last, so the newest is the caller of whatever
// starts next, the function the thread is executing, and the first an end can
// close. The calls are values in one slice, so a call the thread starts
// allocates nothing once the slice has grown to the thread's depth.
//
// arrived is the number the recorder gave the thread when it last joined a
// node. It is read only while the thread is listed at that node: the thread
// leaves a node only to join another, which numbers it again, or to execute
// nothing, which forgets the thread.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: renamed the fields calls, as a test in tests/ needs them (C120a, Q314)
type _Thread struct {
	name    string
	Calls   []_Call
	arrived int
}

// _Call is one call a thread has started and not ended: the function, and
// which call of the run it is.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Call struct {
	callee  *_Callee
	ordinal int
}

// _Innermost is the function of the newest call t is in, or nil when it is in
// none.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-10-29 00:25: reads the thread it is given and returns the function,
//     so a caller looks the thread up once and needs no lookup of the function
//   - 2026-11-10 01:00: now uses Calls, were calls (C120a, Q314)
func (t *_Thread) _Innermost() *_Callee {
	if len(t.Calls) == 0 {
		return nil
	}

	return t.Calls[len(t.Calls)-1].callee
}

// _Newest is the position of the newest call to callee that t is in, or NONE.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-10-29 00:25: compares the function by its pointer, not by its name
//   - 2026-11-10 01:00: now uses Calls, were calls (C120a, Q314)
func (t *_Thread) _Newest(callee *_Callee) int {
	for idx := len(t.Calls) - 1; idx >= 0; idx-- {
		if t.Calls[idx].callee == callee {
			return idx
		}
	}

	return NONE
}

// _Close takes the newest call to callee off t's calls, and returns its
// ordinal, or NONE when t is in none.
//
// The ordinal, not the call: the caller needs only the number, and a call kept
// by value has no pointer to hand out.
//
// Revisions:
//   - 2026-10-02 15:34: initial creation
//   - 2026-10-29 00:25: returns the ordinal, and leaves a thread with no call
//     to the recorder to forget
//   - 2026-11-10 01:00: now uses Calls, were calls (C120a, Q314)
func (t *_Thread) _Close(callee *_Callee) int {
	idx := t._Newest(callee)
	if idx == NONE {
		return NONE
	}

	ordinal := t.Calls[idx].ordinal

	t.Calls = slices.Delete(
		t.Calls,
		idx,
		idx+1,
	)

	return ordinal
}
