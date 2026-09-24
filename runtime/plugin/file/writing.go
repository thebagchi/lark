package file

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"go.starlark.net/starlark"
)

const (
	// NEWLINE separates the lines a writer puts down. It goes between them and
	// not after the last, so what comes back out is what went in.
	NEWLINE = "\n"
)

// ErrLine is returned for something in a list of lines that is not one.
//
// Answers ErrFile too, so a caller branching on the module's general question
// still catches it.
var ErrLine = errors.New("not a line")

// _Writelines puts lines down, replacing what the file held.
//
// Written a line at a time rather than joined first, so putting a million
// lines down costs one buffer rather than the whole text twice.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Writelines(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Putting(fn, args, kwargs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
}

// _Appendlines puts lines down after what the file already held.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Appendlines(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Putting(fn, args, kwargs, os.O_CREATE|os.O_RDWR|os.O_APPEND)
}

// _Putting is the write both line writers do, differing only in how the file
// is opened.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Putting(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	how int,
) (starlark.Value, error) {
	var (
		named string
		given starlark.Iterable
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &named, &given)
	if err != nil {
		return nil, err
	}

	held, err := os.OpenFile(named, how, FILE)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	err = _Joining(held, how)
	if err == nil {
		err = _Pouring(fn, named, held, given)
	}

	closing := held.Close()
	if err == nil && closing != nil {
		err = _Rejected(fn.Name(), named, closing)
	}

	if err != nil {
		return nil, err
	}

	return starlark.None, nil
}

// _Joining puts back the separator that trimming took off, when adding to a
// file that already holds something.
//
// Lines are written with a newline between them and none after the last, so
// what comes back out is what went in. That leaves a file whose final line has
// no terminator, and adding to it would otherwise run the first new line onto
// the end of the old one - measured, ["a", "b"] then ["c"] read back as
// ["a", "bc"].
//
// Does nothing when the file is being replaced rather than added to, when it
// is empty, or when it already ends in a newline someone else put there.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Joining(held *os.File, how int) error {
	if how&os.O_APPEND == 0 {
		return nil
	}

	about, err := held.Stat()
	if err != nil {
		return err
	}

	if about.Size() == 0 {
		return nil
	}

	var last [1]byte

	_, err = held.ReadAt(last[:], about.Size()-1)
	if err != nil {
		return err
	}

	if string(last[:]) == NEWLINE {
		return nil
	}

	_, err = held.WriteString(NEWLINE)

	return err
}

// _Pouring walks the given lines into the open file.
//
// Split from _Putting so that closing the file is not tangled with what goes
// into it: every path out of here still runs the close above.
//
// Revisions:
//   - 2026-09-24 22:58: initial creation
func _Pouring(
	fn *starlark.Builtin,
	named string,
	into *os.File,
	given starlark.Iterable,
) error {
	writer := bufio.NewWriter(into)
	walk := given.Iterate()

	defer walk.Done()

	var (
		line  starlark.Value
		index int
	)

	for walk.Next(&line) {
		held, ok := starlark.AsString(line)
		if !ok {
			return _Rejected(fn.Name(), named,
				fmt.Errorf("line %d is %s: %w", index, line.Type(), ErrLine))
		}

		if index > 0 {
			held = NEWLINE + held
		}

		_, err := writer.WriteString(held)
		if err != nil {
			return _Rejected(fn.Name(), named, err)
		}

		index++
	}

	err := writer.Flush()
	if err != nil {
		return _Rejected(fn.Name(), named, err)
	}

	return nil
}
