// Package disk is internal, not public: it holds the file plugin, the concrete
// Plugin, its calls and its errors.
//
// Package disk holds the file plugin: the concrete Plugin, its calls, its
// names and its errors. Package file forwards to it and hands a host a
// common.Plugin from New.
//
// Revisions:
//   - 2026-11-07 22:40: initial creation, moved from package file (C91w8d)
//   - 2026-11-08 00:06: says it is internal, not public (C91w10, Q242)
package disk

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ErrFile is what every refusal in this module answers to, so a caller
	// asking "did a file call fail" asks once.
	//
	// It is carried rather than wrapped with %w; see _FileError. Its words would
	// otherwise land at the end of every message, where they read as a claim
	// about the filesystem rather than as the category they are - and three of
	// this module's refusals are not the filesystem's doing at all.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ErrFile = errors.New("the filesystem refused")

	// ErrRegular is returned for reading or writing something that is not a
	// regular file: a device, a pipe, a directory.
	//
	// Answers ErrFile as well as itself. A caller branching on "did this call
	// fail in a way I handle" tests the general one, and a refusal that
	// matched only the specific sentinel would slip past every such branch -
	// including the ones written before this sentinel existed, when reading a
	// directory reached the filesystem and came back as ErrFile.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 11:49: the writers return it too (C117, B241)
	//   - 2026-11-10 17:15: renamed from ErrNotAFile (C121, Q349)
	ErrRegular = errors.New("not a regular file")
)

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
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
	//
	// Revisions:
	//   - 2026-09-24 23:30: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STAT        = "stat"
	LINES       = "lines"
	WRITELINES  = "writelines"
	APPENDLINES = "appendlines"

	// PATH and DATA are what the writers call their arguments, so a script
	// may pass them either way round.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	PATH = "path"
	DATA = "data"

	// FILE is the mode a written file takes and DIRECTORY the mode a made
	// directory takes, both as restrictive as this repository's own.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	FILE      = 0o640
	DIRECTORY = 0o750
)

// Plugin is the file module, for a host to hand a compiler with WithPlugins.
// Empty: every call works on the path it is given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Plugin struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-10-02 17:12: a method of Plugin, which a host hands a compiler,
//     rather than of a plugin that registered itself
func (p *Plugin) Name() string {
	return NAME
}

// Values returns the file module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-10-02 17:12: a method of Plugin, which a host hands a compiler,
//     rather than of a plugin that registered itself
func (p *Plugin) Values() starlark.StringDict {
	members := _FileNames()

	for key, value := range _FileStat() {
		members[key] = value
	}

	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name:    NAME,
			Members: members,
		},
	}
}

// _FileNames is the names that read, write, and manage a path.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _FileNames() starlark.StringDict {
	return starlark.StringDict{
		READ: starlark.NewBuiltin(
			NAME+"."+READ,
			_Read,
		),
		BYTES: starlark.NewBuiltin(
			NAME+"."+BYTES,
			_Bytes,
		),
		WRITE: starlark.NewBuiltin(
			NAME+"."+WRITE,
			_Write,
		),
		APPEND: starlark.NewBuiltin(
			NAME+"."+APPEND,
			_Append,
		),
		EXISTS: starlark.NewBuiltin(
			NAME+"."+EXISTS,
			_Exists,
		),
		REMOVE: starlark.NewBuiltin(
			NAME+"."+REMOVE,
			_Remove,
		),
		LIST: starlark.NewBuiltin(
			NAME+"."+LIST,
			_List,
		),
		SIZE: starlark.NewBuiltin(
			NAME+"."+SIZE,
			_Size,
		),
		MKDIR: starlark.NewBuiltin(
			NAME+"."+MKDIR,
			_Mkdir,
		),
	}
}

// _FileStat is the names that report a path or walk its lines.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _FileStat() starlark.StringDict {
	return starlark.StringDict{
		STAT: starlark.NewBuiltin(
			NAME+"."+STAT,
			_Stat,
		),
		LINES: starlark.NewBuiltin(
			NAME+"."+LINES,
			_Reading,
		),
		WRITELINES: starlark.NewBuiltin(
			NAME+"."+WRITELINES,
			_Writelines,
		),
		APPENDLINES: starlark.NewBuiltin(
			NAME+"."+APPENDLINES,
			_Appendlines,
		),
	}
}

// _Read is a whole file as text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-03 00:30: hands the file bytes over, with no copy of them (C37)
func _Read(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Contents(
		thread,
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return starlark.String(unpack.Handover(held)), nil
}

// _Bytes is a whole file as bytes, for one that is not text.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-03 00:30: hands the file bytes over, with no copy of them (C37)
func _Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Contents(
		thread,
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(unpack.Handover(held)), nil
}

// _Contents is a whole file, however the caller means to read it.
//
// A file larger than the free memory is refused (memwatch.ErrRoom) before it
// is read. The size is the one stat gives; a file that grows after the stat, or
// a size-0 file of /proc, is not bounded by it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-01 13:00: reserves no room before the read, the budget being gone
//   - 2026-11-03 00:36: refuses a file that does not fit the free memory (C46)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
//   - 2026-11-11 06:43: reads the file it opened with _Opened, which checks it (C122, B272)
func _Contents(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]byte, error) {
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

	size, err := _Regular(
		fn.Name(),
		named,
	)
	if err != nil {
		return nil, err
	}

	err = memwatch.Room(
		fn.Name(),
		uint64(size),
	)
	if err != nil {
		return nil, err
	}

	opened, err := _Opened(
		fn.Name(),
		named,
		os.O_RDONLY,
	)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = opened.Close()
	}()

	held, err := _Slurped(
		opened,
		size,
	)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return held, nil
}

// _Slurped is everything left in an open file, read into a buffer sized from
// the size fstat gave, as os.ReadFile sizes its own. A file that grows after the
// fstat is read to its end.
//
// Revisions:
//   - 2026-11-11 06:43: initial creation, the read os.ReadFile did (C122, B272)
func _Slurped(
	opened *os.File,
	size int64,
) ([]byte, error) {
	const MINIMUM = 512

	size++
	if size < MINIMUM {
		size = MINIMUM
	}

	data := make(
		[]byte,
		0,
		size,
	)

	for {
		got, err := opened.Read(data[len(data):cap(data)])
		data = data[:len(data)+got]

		if errors.Is(
			err,
			io.EOF,
		) {
			return data, nil
		}

		if err != nil {
			return nil, err
		}

		if len(data) == cap(data) {
			data = append(
				data[:cap(data)],
				0,
			)[:len(data)]
		}
	}
}

// _Write puts data in a file, replacing what was there.
//
// The directory above it is made if it is not there, because a script writing
// a report into a place it also chose should not have to make the place in a
// second call.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 11:48: refuses a path that is not a regular file before the
//     open (C117, B241)
//   - 2026-11-11 00:36: stores with _Stored, which checks the open file (C122, B266)
func _Write(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	named, data, err := _Destination(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(
		filepath.Dir(named),
		DIRECTORY,
	)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return _Stored(
		fn,
		named,
		data,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
	)
}

// _Append puts data at the end of a file, making it if it is not there.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-11 00:36: stores with _Stored (C122, B266)
func _Append(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	named, data, err := _Destination(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(
		filepath.Dir(named),
		DIRECTORY,
	)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return _Stored(
		fn,
		named,
		data,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
	)
}

// _Stored opens the file as how says, writes data, and closes it.
//
// A failed write is still closed, and that failure is the one reported. A path
// that is not a regular file is refused by _Opened before anything is written.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 11:48: refuses a path that is not a regular file before the
//     open (C117, B241)
//   - 2026-11-11 00:36: renamed from _Appending, which only appended; takes how, and opens
//     with _Opened, which checks the open file (C122, B266)
func _Stored(
	fn *starlark.Builtin,
	named string,
	data []byte,
	how int,
) (starlark.Value, error) {
	held, err := _Opened(
		fn.Name(),
		named,
		how,
	)
	if err != nil {
		return nil, err
	}

	_, err = held.Write(data)
	if err != nil {
		_ = held.Close()

		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	err = held.Close()
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
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

	_, err = os.Stat(named)
	if err == nil {
		return starlark.True, nil
	}

	if errors.Is(
		err,
		os.ErrNotExist,
	) {
		return starlark.False, nil
	}

	return nil, _Refused(
		fn.Name(),
		err,
	)
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

	err = os.Remove(named)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return starlark.None, nil
}

// _List is the names in a directory, sorted.
//
// Names rather than paths, and sorted rather than in whatever order the
// filesystem keeps them, so that a script reading a directory twice reads it
// the same way.
//
// The order is os.ReadDir's own: it returns "all its directory entries sorted
// by filename", by the bytes of the name. Sorting again would change nothing.
// On an error, os.ReadDir may return the entries read so far; this refuses and
// drops them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 08:27: takes the order os.ReadDir gives, rather than sorting
//     the names again
func _List(
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

	entries, err := os.ReadDir(named)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return starlark.NewList(_Names(entries)), nil
}

// _Names is the entry names, in the order given, as Starlark strings.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation, as _Sorted
//   - 2026-11-02 08:27: one pass over the entries, with no second sort and no
//     slice of plain names
func _Names(entries []os.DirEntry) []starlark.Value {
	held := make(
		[]starlark.Value,
		0,
		len(entries),
	)

	for _, entry := range entries {
		held = append(
			held,
			starlark.String(entry.Name()),
		)
	}

	return held
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

	held, err := os.Stat(named)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
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

	err = os.MkdirAll(
		named,
		DIRECTORY,
	)
	if err != nil {
		return nil, _Refused(
			fn.Name(),
			err,
		)
	}

	return starlark.None, nil
}

// _Destination reads the path and the data every writer takes.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-03 23:00: reads a view of the data, with no copy: the writers
//     only read it, and keep none of it past the call (C38b, plan D4)
func _Destination(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, []byte, error) {
	var (
		named string
		given starlark.Value
	)

	err := starlark.UnpackArgs(
		fn.Name(),
		args,
		kwargs,
		PATH,
		&named,
		DATA,
		&given,
	)
	if err != nil {
		return "", nil, err
	}

	data, err := unpack.View(
		fn.Name(),
		given,
	)
	if err != nil {
		return "", nil, err
	}

	return named, data, nil
}
