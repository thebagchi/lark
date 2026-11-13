package disk

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// LINES_TYPE is what a script's type() reports for what lines returns.
	//
	// Revisions:
	//   - 2026-09-24 23:30: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	LINES_TYPE = "lines"
)

// ErrWalked is returned for reading a file that has already been read.
//
// A walk is one pass over one file, as a file object is in Python. Reading it
// twice would quietly start again from the beginning, which is not what
// anyone holding it a second time meant - and if two threads hold it, not
// something either of them could have predicted.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrWalked = errors.New("already read")
)

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
//   - 2026-11-01 13:00: takes no budget
//   - 2026-11-03 00:37: reads _Regular's size and leaves it, for lines needs none (C46)
func _Reading(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var named string

	err := starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		kwargs,
		1,
		&named,
	)
	if err != nil {
		return nil, err
	}

	_, err = _Regular(
		fn.Name(),
		named,
	)
	if err != nil {
		return nil, err
	}

	return &_Lines{
		thread: thread,
		named:  named,
	}, nil
}

// _Lines is the value lines hands back.
//
// It holds the thread that asked for it because a read that fails partway
// through a loop has nowhere to return an error to, and ending the run is the
// only answer left; see _Walk.Done.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Lines struct {
	thread *starlark.Thread
	named  string
	walked atomic.Bool
}

// String names the file this walks, with a path of more than 100 bytes cut
// (Q4).
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
//   - 2026-11-11 06:43: cuts the path with unpack.ShortText (C122, B273)
func (l *_Lines) String() string {
	return fmt.Sprintf(
		"<%s %s>",
		LINES_TYPE,
		unpack.ShortText(l.named),
	)
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
	return 0, fmt.Errorf(
		"unhashable: %s",
		LINES_TYPE,
	)
}

// Iterate opens the file and walks it, once.
//
// A line may be as long as the file is: the scanner is given the largest
// limit it takes, so no number chosen here refuses a long line.
//
// The claim on the one walk is taken atomically, so two threads racing to read
// the same value cannot both win.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
//   - 2026-11-01 13:00: a line may be as long as the scanner allows, the
//     run's budget being gone
//   - 2026-11-03 21:50: splits through _Split, so a line that would not fit is
//     refused (C46 fix round 1)
//   - 2026-11-11 06:43: opens the file with _Opened, which checks what it opened (C122, B272)
func (l *_Lines) Iterate() starlark.Iterator {
	if !l.walked.CompareAndSwap(
		false,
		true,
	) {
		return &_Walk{
			lines:  l,
			failed: ErrWalked,
		}
	}

	held, err := _Opened(
		NAME+"."+LINES,
		l.named,
		os.O_RDONLY,
	)
	if err != nil {
		return &_Walk{
			lines:  l,
			failed: err,
		}
	}

	reader := bufio.NewScanner(held)
	reader.Buffer(
		nil,
		math.MaxInt,
	)
	reader.Split(_Split())

	return &_Walk{
		lines:  l,
		file:   held,
		reader: reader,
	}
}

// _Split is bufio.ScanLines, and a refusal where the line has no end in the
// data read so far.
//
// The scanner's limit is MaxInt, so one line with no newline grows its buffer,
// doubling each time, to the size of the file. When the scanner asks for more,
// its next buffer is GROWTH times the data it holds; Room is asked about that
// size first, so the line ends with memwatch.ErrRoom and not with the
// process. Room reads no memory for a size under common.ROOM_SKIP, so ordinary
// lines cost one compare.
//
// A function that returns the split, because bufio.SplitFunc fixes a bool
// parameter that the flag-parameter rule would refuse in a declaration.
//
// Revisions:
//   - 2026-11-03 21:50: initial creation (C46 fix round 1)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func _Split() bufio.SplitFunc {
	const GROWTH = 2

	return func(
		data []byte,
		atEOF bool,
	) (int, []byte, error) {
		advance, token, err := bufio.ScanLines(
			data,
			atEOF,
		)
		if err != nil || advance > 0 || atEOF {
			return advance, token, err
		}

		err = memwatch.Room(
			NAME+"."+LINES,
			uint64(len(data))*GROWTH,
		)

		return 0, nil, err
	}
}

// _Walk is one pass over one file.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Walk struct {
	lines  *_Lines
	file   *os.File
	reader *bufio.Scanner
	failed error
}

// Next puts the next line into p, or stops.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
//   - 2026-11-01 13:00: charges nothing for the line, the budget being gone
func (w *_Walk) Next(p *starlark.Value) bool {
	if w.failed != nil {
		return false
	}

	if !w.reader.Scan() {
		w.failed = w.reader.Err()

		return false
	}

	*p = starlark.String(w.reader.Text())

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
//   - 2026-11-01 13:00: credits nothing, the budget being gone
func (w *_Walk) Done() {
	if w.file != nil {
		err := w.file.Close()
		if err != nil && w.failed == nil {
			w.failed = err
		}
	}

	if w.failed == nil {
		return
	}

	cause := _Rejected(
		NAME+"."+LINES,
		w.lines.named,
		w.failed,
	)

	// Fail returns its cause so a builtin can raise it. Done has nothing to
	// raise to, so what comes back is kept as the reason this walk stopped:
	// the walk is the only thing left that can still be asked.
	w.failed = scheduler.Fail(
		w.lines.thread,
		cause,
	)
}
