// Package file gives a script the filesystem. A host enables it by handing a
// compiler its Plugin, and nothing else does: it is not one of the runtime's
// own plugins, which every script has.
//
// That is the whole of the warning. Every other plugin works on what a script
// was handed; this one reaches whatever the host process can reach, with the
// host's own permissions, and a script is free to name any path it likes. A
// host that runs scripts it did not write does not hand it over, and not
// handing it over is the whole of leaving it out.
//
// Paths are the host's own, through path/filepath, and are not interpreted
// here beyond what the operating system does with them. Relative ones resolve
// against the process's working directory, which is the host's business and
// not this package's to invent.
//
// Text and bytes are separate calls rather than one call with a flag: read
// answers with a str and bytes answers with a str's worth of bytes, and which
// one a script wants is known when it is written.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 22:40: the code moved to file/internal/disk; New hands a host the
//     plugin, and the errors forward (C91w8d)
package file

import (
	"github.com/thebagchi/lark/v1/plugin/file/internal/disk"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// ErrFile is what every refusal in this module answers to, so a caller
// asking "did a file call fail" asks once.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 22:40: an alias of disk.ErrFile; the earlier revisions are there (C91w8d)
var (
	ErrFile = disk.ErrFile
)

// ErrRegular is returned for reading or writing something that is not a
// regular file: a device, a pipe, a directory. It answers ErrFile as well as
// itself.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 22:40: an alias of disk.ErrNotAFile; the earlier revisions are there (C91w8d)
//   - 2026-11-10 11:49: write, append, writelines and appendlines return it too (C117, B241)
//   - 2026-11-10 17:15: renamed from ErrNotAFile (C121, Q349)
var (
	ErrRegular = disk.ErrRegular
)

// ErrWalked is returned for reading a file that has already been read.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 22:40: an alias of disk.ErrWalked; the earlier revisions are there (C91w8d)
var (
	ErrWalked = disk.ErrWalked
)

// ErrLine is returned for something in a list of lines that is not one. It
// answers ErrFile too.
//
// Revisions:
//   - 2026-09-24 23:30: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 22:40: an alias of disk.ErrLine; the earlier revisions are there (C91w8d)
var (
	ErrLine = disk.ErrLine
)

// New returns the file plugin, for a host to hand a compiler with
// WithPlugins. Empty: every call works on the path it is given.
//
// Revisions:
//   - 2026-11-07 22:40: initial creation, in place of &file.Plugin{} (C91w8d, Q105)
func New() common.Plugin {
	return new(disk.Plugin)
}
