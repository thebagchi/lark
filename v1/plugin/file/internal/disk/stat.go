package disk

import (
	"errors"
	"os"
	"syscall"

	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// The fields a stat answers with, beside SIZE, which a stat calls what
// the builtin of that name is already called.
//
// MODE is the permission bits as a number, the way Python's stat.S_IMODE
// gives them, so that a script testing one masks rather than parses:
// info.mode & 0o700. Printing it is "%o" % info.mode, because Starlark
// has neither oct() nor a width in its interpolation.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	DIR      = "dir"
	MODE     = "mode"
	MODIFIED = "modified"
)

// _Stat is everything the stat syscall read, rather than the one field the
// caller asked for.
//
// size and exists both call os.Stat and discard all of it but one answer, so
// a script wanting a file's size and its type made the same syscall twice and
// could be told two things that were never true at the same moment.
//
// modified is the time module's own value rather than a number, because
// from_timestamp takes whole seconds and a float would have lost what it was
// given before the script could convert it.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Stat(
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

	about, err := os.Stat(named)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	held := starlark.StringDict{
		SIZE:     starlark.MakeInt64(about.Size()),
		DIR:      starlark.Bool(about.IsDir()),
		MODE:     starlark.MakeUint64(uint64(about.Mode().Perm())),
		MODIFIED: starlarktime.Time(about.ModTime()),
	}

	return starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		held,
	), nil
}

// _Regular refuses a path that names anything but a regular file, and returns
// its size in bytes.
//
// Split from the readers because a read of a pipe or a device may never end,
// and the refusal is the same wherever it is asked.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
//   - 2026-11-01 13:00: renamed from _Sized and returns no size, the budget
//     that wanted the size being gone
//   - 2026-11-03 00:36: returns the size again, for the size guard of read and bytes (C46)
//   - 2026-11-10 17:15: uses ErrRegular, formerly ErrNotAFile (C121, Q349)
func _Regular(
	name string,
	named string,
) (int64, error) {
	about, err := os.Stat(named)
	if err != nil {
		return 0, _Refused(
			name,
			err,
		)
	}

	if !about.Mode().IsRegular() {
		return 0, _Rejected(
			name,
			named,
			ErrRegular,
		)
	}

	return about.Size(), nil
}

// _Opened opens a path, for reading or writing as how says, and refuses it when
// what it opened is not a regular file. A path that names nothing is made, if
// how says so.
//
// The check is made on the open file, with fstat, and not on the path before
// the open: a path can be swapped for a named pipe between a stat and an open,
// and opening a pipe waits for the other end, which cancelling the run does not
// stop (B241, B266, B272). The open is O_NONBLOCK, so it never waits: a pipe
// nobody reads is refused by the system with ENXIO when it is opened for
// writing, and any other pipe is opened and refused here. O_NOCTTY keeps a
// terminal from becoming the controlling terminal of the process. Neither flag
// changes how a regular file is read or written. A directory is refused by the
// system with EISDIR when it is opened for writing, and here otherwise.
//
// Returns ErrRegular for anything that is not a regular file, and the open file
// otherwise. The caller closes it.
//
// Revisions:
//   - 2026-11-11 06:43: initial creation, replacing _Writable, which checked the path before
//     the open; opens for reading too (C122, B266, B272)
func _Opened(
	name string,
	named string,
	how int,
) (*os.File, error) {
	held, err := os.OpenFile(
		named,
		how|syscall.O_NONBLOCK|syscall.O_NOCTTY,
		FILE,
	)
	if errors.Is(
		err,
		syscall.ENXIO,
	) || errors.Is(
		err,
		syscall.EISDIR,
	) {
		return nil, _Rejected(
			name,
			named,
			ErrRegular,
		)
	}

	if err != nil {
		return nil, _Refused(
			name,
			err,
		)
	}

	about, err := held.Stat()
	if err != nil {
		_ = held.Close()

		return nil, _Refused(
			name,
			err,
		)
	}

	if !about.Mode().IsRegular() {
		_ = held.Close()

		return nil, _Rejected(
			name,
			named,
			ErrRegular,
		)
	}

	return held, nil
}
