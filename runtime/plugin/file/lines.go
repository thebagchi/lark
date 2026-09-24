package file

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/scheduler"
)

const (
	// LINES_TYPE is what a script's type() reports for what lines returns.
	LINES_TYPE = "lines"
)

// ErrWalked is returned for reading a file that has already been read.
//
// A walk is one pass over one file, as a file object is in Python. Reading it
// twice would quietly start again from the beginning, which is not what
// anyone holding it a second time meant - and if two threads hold it, not
// something either of them could have predicted.
var ErrWalked = errors.New("already read")

// _Reading is a file a script walks one line at a time.
//
// read sizes its buffer from the file, so a file too big to hold is a process
// too big to keep: measured, 512MB of file peaked at 1057MB of memory. Walking
// costs one line at a time, which is why this needs no limit on how big a file
// may be.
//
// The stat happens here, at the call that named the file, so a missing file or
// a directory is refused where it was written rather than in a loop further
// down. The handle waits for the loop, so naming a file and never reading it
// leaves nothing open.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Reading(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var named string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &named)
	if err != nil {
		return nil, err
	}

	_, err = _Sized(fn.Name(), named)
	if err != nil {
		return nil, err
	}

	budget := scheduler.Allowance(thread)

	return &_Lines{thread: thread, named: named, budget: budget}, nil
}

// _Lines is the value lines hands back.
//
// It holds the thread that asked for it because a read that fails partway
// through a loop has nowhere to return an error to, and ending the run is the
// only answer left; see _Walk.Done.
type _Lines struct {
	thread *starlark.Thread
	named  string
	budget *scheduler.Budget
	walked atomic.Bool
}

// String names the file this walks.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) String() string {
	return fmt.Sprintf("<%s %s>", LINES_TYPE, l.named)
}

// Type is what a script's type() reports.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) Type() string {
	return LINES_TYPE
}

// Freeze is empty.
//
// Nothing a script can see changes: the path is fixed at the call, and the one
// thing that does change - whether this has been read - is held atomically, so
// freezing it would not make it safer than it already is.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) Freeze() {
}

// Truth is always true, because the file was there when it was named.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) Truth() starlark.Bool {
	return starlark.True
}

// Hash refuses, as a file is not a key.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable: %s", LINES_TYPE)
}

// Iterate opens the file and walks it, once.
//
// How long a line may be is what the run has left to spend rather than a
// number chosen here, so a file of one enormous line is refused for the reason
// it deserves - the memory it wanted - and not for tripping a limit nobody
// chose.
//
// The claim on the one walk is taken atomically, so two threads racing to read
// the same value cannot both win.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (l *_Lines) Iterate() starlark.Iterator {
	if !l.walked.CompareAndSwap(false, true) {
		return &_Walk{lines: l, failed: ErrWalked}
	}

	held, err := os.Open(l.named)
	if err != nil {
		return &_Walk{lines: l, failed: err}
	}

	reader := bufio.NewScanner(held)
	reader.Buffer(nil, int(l.budget.Left()))

	return &_Walk{lines: l, file: held, reader: reader}
}

// _Walk is one pass over one file.
type _Walk struct {
	lines   *_Lines
	file    *os.File
	reader  *bufio.Scanner
	failed  error
	charged int64
}

// Next puts the next line into p, or stops.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (w *_Walk) Next(p *starlark.Value) bool {
	if w.failed != nil {
		return false
	}

	if !w.reader.Scan() {
		w.failed = _Why(w.reader.Err(), w.lines.budget)

		return false
	}

	held := w.reader.Text()

	// Charged before the line before it is given back, so the reserve never
	// dips below what is actually held. That costs one line's headroom, which
	// is the thing being bounded anyway.
	err := w.lines.budget.Charge(int64(len(held)))
	if err != nil {
		w.failed = err

		return false
	}

	w.lines.budget.Credit(w.charged)
	w.charged = int64(len(held))

	*p = starlark.String(held)

	return true
}

// Done closes the file, and ends the run if the walk did not finish.
//
// Starlark's Iterator returns an error from neither of its methods, so a disk
// that fails halfway through a loop has nowhere to report it. Stopping quietly
// would hand the script a truncated file it believes it read whole, which is
// the failure this exists to remove. Ending the run is the only answer that
// cannot be mistaken for success.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func (w *_Walk) Done() {
	w.lines.budget.Credit(w.charged)
	w.charged = 0

	if w.file != nil {
		err := w.file.Close()
		if err != nil && w.failed == nil {
			w.failed = err
		}
	}

	if w.failed == nil {
		return
	}

	cause := _Rejected(NAME+"."+LINES, w.lines.named, w.failed)

	// Fail returns its cause so a builtin can raise it. Done has nothing to
	// raise to, so what comes back is kept as the reason this walk stopped:
	// the walk is the only thing left that can still be asked.
	w.failed = scheduler.Fail(w.lines.thread, cause)
}

// _Why is what stopped a scan, said in the terms the caller set it in.
//
// The scanner is given the run's remaining budget as its limit, so a line it
// calls too long is a line the run could not afford. Reporting the library's
// own wording would blame the file for a ceiling this package chose.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Why(failed error, budget *scheduler.Budget) error {
	if !errors.Is(failed, bufio.ErrTooLong) {
		return failed
	}

	return fmt.Errorf("a line longer than the %d bytes left: %w",
		budget.Left(), scheduler.ErrMemory)
}
