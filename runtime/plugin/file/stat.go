package file

import (
	"fmt"
	"os"

	starlarktime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	// The fields a stat answers with, beside SIZE, which a stat calls what
	// the builtin of that name is already called.
	//
	// MODE is the permission bits as a number, the way Python's stat.S_IMODE
	// gives them, so that a script testing one masks rather than parses:
	// info.mode & 0o700. Printing it is "%o" % info.mode, because Starlark
	// has neither oct() nor a width in its interpolation.
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

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &named)
	if err != nil {
		return nil, err
	}

	about, err := os.Stat(named)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	held := starlark.StringDict{
		SIZE:     starlark.MakeInt64(about.Size()),
		DIR:      starlark.Bool(about.IsDir()),
		MODE:     starlark.MakeUint64(uint64(about.Mode().Perm())),
		MODIFIED: starlarktime.Time(about.ModTime()),
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, held), nil
}

// _Sized is what a file holds, for a caller deciding whether it can afford to
// read it.
//
// Split from the readers because they need the number before they allocate,
// and because a caller that only wants the size should not pay for a read.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Sized(name string, named string) (os.FileInfo, error) {
	about, err := os.Stat(named)
	if err != nil {
		return nil, _Refused(name, err)
	}

	if !about.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %s: %w: %w", name, named, ErrNotAFile, ErrFile)
	}

	return about, nil
}
