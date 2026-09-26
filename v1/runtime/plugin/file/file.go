// Package file gives a script the filesystem. Importing it is what enables
// it.
//
// That sentence is the whole of the warning. Every other plugin here works on
// what a script was handed; this one reaches whatever the host process can
// reach, with the host's own permissions, and a script is free to name any
// path it likes. A host that runs scripts it did not write should not import
// this package - and because importing is what enables a plugin, not importing
// it is the whole of leaving it out.
//
// Paths are the host's own, through path/filepath, and are not interpreted
// here beyond what the operating system does with them. Relative ones resolve
// against the process's working directory, which is the host's business and
// not this package's to invent.
//
// Text and bytes are separate calls rather than one call with a flag: read
// answers with a str and bytes answers with a str's worth of bytes, and which
// one a script wants is known when it is written.
package file

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrFile is what every refusal in this module answers to, so a caller
	// asking "did a file call fail" asks once.
	//
	// It is carried rather than wrapped with %w; see _Failed. Its words would
	// otherwise land at the end of every message, where they read as a claim
	// about the filesystem rather than as the category they are - and three of
	// this module's refusals are not the filesystem's doing at all.
	ErrFile = errors.New("the filesystem refused")

	// ErrNotAFile is returned for reading something that is not a regular
	// file: a device, a pipe, a directory.
	//
	// Answers ErrFile as well as itself. A caller branching on "did this call
	// fail in a way I handle" tests the general one, and a refusal that
	// matched only the specific sentinel would slip past every such branch -
	// including the ones written before this sentinel existed, when reading a
	// directory reached the filesystem and came back as ErrFile.
	ErrNotAFile = errors.New("not a regular file")
)

const (
	// NAME is the module, and the names it holds.
	NAME   = "file"
	READ   = "read"
	BYTES  = "bytes"
	WRITE  = "write"
	APPEND = "append"
	EXISTS = "exists"
	REMOVE = "remove"
	LIST   = "list"
	SIZE   = "size"
	MKDIR  = "mkdir"

	// The line oriented half of the module, and the stat that keeps what the
	// syscall read.
	STAT        = "stat"
	LINES       = "lines"
	WRITELINES  = "writelines"
	APPENDLINES = "appendlines"

	// PATH and DATA are what the writers call their arguments, so a script
	// may pass them either way round.
	PATH = "path"
	DATA = "data"

	// FILE is the mode a written file takes and DIRECTORY the mode a made
	// directory takes, both as restrictive as this repository's own.
	FILE      = 0o640
	DIRECTORY = 0o750

	// _STRING is what a read pays on top of the bytes it read, because the
	// file and the string copied from it are both live at once.
	_STRING = 2
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func init() {
	plugin.Register(new(_File))
}

// _File is the plugin. Empty: every call works on the path it is given.
type _File struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (f *_File) Name() string {
	return NAME
}

// Values returns the file module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (f *_File) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				READ:   starlark.NewBuiltin(NAME+"."+READ, _Read),
				BYTES:  starlark.NewBuiltin(NAME+"."+BYTES, _Bytes),
				WRITE:  starlark.NewBuiltin(NAME+"."+WRITE, _Write),
				APPEND: starlark.NewBuiltin(NAME+"."+APPEND, _Append),
				EXISTS: starlark.NewBuiltin(NAME+"."+EXISTS, _Exists),
				REMOVE: starlark.NewBuiltin(NAME+"."+REMOVE, _Remove),
				LIST:   starlark.NewBuiltin(NAME+"."+LIST, _List),
				SIZE:   starlark.NewBuiltin(NAME+"."+SIZE, _Size),
				MKDIR:  starlark.NewBuiltin(NAME+"."+MKDIR, _Mkdir),

				STAT:       starlark.NewBuiltin(NAME+"."+STAT, _Stat),
				LINES:      starlark.NewBuiltin(NAME+"."+LINES, _Reading),
				WRITELINES: starlark.NewBuiltin(NAME+"."+WRITELINES, _Writelines),
				APPENDLINES: starlark.NewBuiltin(
					NAME+"."+APPENDLINES,
					_Appendlines,
				),
			},
		},
	}
}

// _Read is a whole file as text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Read(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Contents(thread, fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(held), nil
}

// _Bytes is a whole file as bytes, for one that is not text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Contents(thread, fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(held), nil
}

// _Contents is a whole file, however the caller means to read it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Contents(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]byte, error) {
	var named string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &named)
	if err != nil {
		return nil, err
	}

	about, err := _Sized(fn.Name(), named)
	if err != nil {
		return nil, err
	}

	budget := scheduler.Allowance(thread)

	// Charged from the stat, before anything is allocated, so a file this run
	// cannot afford costs one syscall rather than an allocation that takes the
	// process down with it. Twice the size, because ReadFile allocates the
	// bytes and starlark.String copies them: measured, 512MB of file peaked at
	// 1057MB of memory.
	size := about.Size() * _STRING

	err = budget.Charge(size)
	if err != nil {
		return nil, _Rejected(fn.Name(), named, err)
	}

	// Given back when the value is handed over: from then on the script owns
	// it and Go's collector decides when it goes. This bounds one read, and
	// every read in flight at once, not the whole of what a script is holding.
	defer budget.Credit(size)

	held, err := os.ReadFile(named)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return held, nil
}

// _Write puts data in a file, replacing what was there.
//
// The directory above it is made if it is not there, because a script writing
// a report into a place it also chose should not have to make the place in a
// second call.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Write(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	named, data, err := _Destination(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(named), DIRECTORY)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	err = os.WriteFile(named, data, FILE)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return starlark.None, nil
}

// _Append puts data at the end of a file, making it if it is not there.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Append(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	named, data, err := _Destination(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(named), DIRECTORY)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	held, err := os.OpenFile(named, os.O_CREATE|os.O_WRONLY|os.O_APPEND, FILE)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	_, err = held.Write(data)
	if err != nil {
		// Closed anyway, and the write's failure is the one worth reporting.
		_ = held.Close()

		return nil, _Refused(fn.Name(), err)
	}

	err = held.Close()
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return starlark.None, nil
}

// _Exists reports whether there is anything at a path.
//
// A bool rather than a refusal, because asking is what a caller does when they
// do not know - and a question that fails when the answer is no is not a
// question.
//
// Only absence is False. Every other refusal is an error: a directory this
// process may not search answers "I cannot tell", and reporting that as False
// says the file is not there when it is. A script deciding whether to write
// would then overwrite something it could not see.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-09-24 16:08: tells absence from being unable to look
func _Exists(
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

	_, err = os.Stat(named)
	if err == nil {
		return starlark.True, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return starlark.False, nil
	}

	return nil, _Refused(fn.Name(), err)
}

// _Remove deletes a file, or an empty directory.
//
// One thing, never a tree. A script that meant to remove one file and named a
// directory by mistake should get a refusal rather than an afternoon's work
// undone.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Remove(
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

	err = os.Remove(named)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return starlark.None, nil
}

// _List is the names in a directory, sorted.
//
// Names rather than paths, and sorted rather than in whatever order the
// filesystem keeps them, so that a script reading a directory twice reads it
// the same way.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _List(
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

	entries, err := os.ReadDir(named)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	sort.Strings(names)

	held := make([]starlark.Value, 0, len(names))

	for _, name := range names {
		held = append(held, starlark.String(name))
	}

	return starlark.NewList(held), nil
}

// _Size is how many bytes a file holds.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Size(
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

	held, err := os.Stat(named)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return starlark.MakeInt64(held.Size()), nil
}

// _Mkdir makes a directory, and every directory above it that is missing.
//
// Making one that is already there is not a failure, as it is not for the
// command line tool of the same name given the same instruction.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Mkdir(
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

	err = os.MkdirAll(named, DIRECTORY)
	if err != nil {
		return nil, _Refused(fn.Name(), err)
	}

	return starlark.None, nil
}

// _Destination reads the path and the data every writer takes.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Destination(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, []byte, error) {
	var (
		named string
		given starlark.Value
	)

	err := starlark.UnpackArgs(fn.Name(), args, kwargs, PATH, &named, DATA, &given)
	if err != nil {
		return "", nil, err
	}

	data, err := unpack.Bytes(fn.Name(), given)
	if err != nil {
		return "", nil, err
	}

	return named, data, nil
}
