package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// MEMINFO is where the kernel lists system memory, in KILOBYTEs.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	MEMINFO = "/proc/meminfo"

	// CGROUP is the cgroup v2 mount.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	CGROUP = "/sys/fs/cgroup"

	// SELF is the file naming this process's place in the cgroup tree.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	SELF = "/proc/self/cgroup"

	// STATUS is the file naming this process's mapped address space (VmSize).
	//
	// Revisions:
	//   - 2026-11-03 22:33: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STATUS = "/proc/self/status"

	// MARKER is the file a cgroup v2 mount has at its root.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	MARKER = "cgroup.controllers"

	// KILOBYTE is how many bytes a kB of /proc/meminfo is.
	//
	// Revisions:
	//   - 2026-11-02 23:59: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	KILOBYTE = 1024
)

// System returns the source the watch reads when none is injected: the tighter
// of the machine and this process's cgroup, counting any use.
//
// The machine's use is MemTotal less MemAvailable, the kernel's "estimate of
// how much memory is available for starting new applications, without
// swapping" (proc_meminfo(5)), so page cache the kernel can drop is not use.
// The cgroup's use is memory.current less inactive_file for the same reason.
// The tighter is the one with the larger share in use: the kernel's OOM killer
// acts on the cgroup's memory.max (cgroup-v2 documentation), and the machine's
// figure predicts the kill when the cgroup has no limit.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func System() func() (*common.MemoryReading, error) {
	return func() (*common.MemoryReading, error) {
		return Read(
			MEMINFO,
			CGROUP,
			SELF,
		)
	}
}

// Read reads the machine at meminfo and the cgroup at root and self, and
// returns the one with the larger share in use. A cgroup that cannot be read or
// has no limit leaves the machine's reading.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Read(
	meminfo string,
	root string,
	self string,
) (*common.MemoryReading, error) {
	machine, err := Machine(meminfo)
	if err != nil {
		return nil, err
	}

	limited, err := Cgroup(
		root,
		self,
	)
	if err != nil || limited == nil {
		return machine, nil
	}

	return Tighter(
		machine,
		limited,
	), nil
}

// Tighter returns the reading with the larger share of its total in use.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Tighter(
	a *common.MemoryReading,
	b *common.MemoryReading,
) *common.MemoryReading {
	if float64(b.Used)*float64(a.Total) > float64(a.Used)*float64(b.Total) {
		return b
	}

	return a
}

// Machine reads MemTotal and MemAvailable from the meminfo file at path.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Machine(path string) (*common.MemoryReading, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: cannot read %s: %w",
			ErrUnavailable,
			path,
			err,
		)
	}

	var (
		total, available         uint64
		haveTotal, haveAvailable bool
	)

	for line := range strings.SplitSeq(
		string(data),
		"\n",
	) {
		name, rest, found := strings.Cut(
			line,
			":",
		)
		if !found {
			continue
		}

		switch name {
		case "MemTotal":
			total, haveTotal = _Kilobytes(rest)

		case "MemAvailable":
			available, haveAvailable = _Kilobytes(rest)
		}
	}

	if !haveTotal || !haveAvailable || available > total {
		return nil, fmt.Errorf(
			"%w: %s has no usable MemTotal and MemAvailable",
			ErrUnavailable,
			path,
		)
	}

	return &common.MemoryReading{
		Used:  total - available,
		Total: total,
	}, nil
}

// _Kilobytes reads " 16379576 kB" as bytes.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Kilobytes(text string) (uint64, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0, false
	}

	number, err := strconv.ParseUint(
		fields[0],
		10,
		64,
	)
	if err != nil || number > ^uint64(0)/KILOBYTE {
		return 0, false
	}

	return number * KILOBYTE, true
}

// Cgroup reads the limit and the use of this process's own cgroup, found
// through the file at self under the mount at root. It returns nil, with no
// error, when that cgroup has no limit: a memory.max of "max", or no memory.max
// file. Use is memory.current less inactive_file from memory.stat.
//
// Only the process's own cgroup is read, not its ancestors, so a limit set
// higher up the tree is missed and the machine's reading stands.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Cgroup(
	root string,
	self string,
) (*common.MemoryReading, error) {
	line, err := os.ReadFile(self)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: cannot read %s: %w",
			ErrUnavailable,
			self,
			err,
		)
	}

	var (
		place string
		found bool
	)

	for text := range strings.SplitSeq(
		string(line),
		"\n",
	) {
		place, found = strings.CutPrefix(
			strings.TrimSpace(text),
			"0::",
		)
		if found {
			break
		}
	}

	if !found {
		return nil, fmt.Errorf(
			"%w: %s has no cgroup v2 line",
			ErrUnavailable,
			self,
		)
	}

	return _Limited(root + place)
}

// _Limited reads the memory files of the cgroup directory dir.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Limited(dir string) (*common.MemoryReading, error) {
	text, err := os.ReadFile(dir + "/memory.max")
	if errors.Is(
		err,
		fs.ErrNotExist,
	) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf(
			"%w: cannot read %s/memory.max: %w",
			ErrUnavailable,
			dir,
			err,
		)
	}

	limit := strings.TrimSpace(string(text))
	if limit == "max" {
		return nil, nil
	}

	total, err := strconv.ParseUint(
		limit,
		10,
		64,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %s/memory.max is not a number or max",
			ErrUnavailable,
			dir,
		)
	}

	current, err := _Number(dir + "/memory.current")
	if err != nil {
		return nil, err
	}

	cache, err := _Cache(dir + "/memory.stat")
	if err != nil {
		return nil, err
	}

	return &common.MemoryReading{
		Used: current - min(
			cache,
			current,
		),
		Total: total,
	}, nil
}

// _Number reads a file holding one unsigned number.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Number(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: cannot read %s: %w",
			ErrUnavailable,
			path,
			err,
		)
	}

	number, err := strconv.ParseUint(
		strings.TrimSpace(string(data)),
		10,
		64,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: %s is not a number",
			ErrUnavailable,
			path,
		)
	}

	return number, nil
}

// _Cache reads inactive_file from a memory.stat file: page cache the kernel can
// drop, which is not memory the process holds. A file without the key is zero.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func _Cache(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: cannot read %s: %w",
			ErrUnavailable,
			path,
			err,
		)
	}

	for line := range strings.SplitSeq(
		string(data),
		"\n",
	) {
		name, value, found := strings.Cut(
			line,
			" ",
		)
		if !found || name != "inactive_file" {
			continue
		}

		number, err := strconv.ParseUint(
			value,
			10,
			64,
		)
		if err != nil {
			return 0, fmt.Errorf(
				"%w: %s has a bad inactive_file",
				ErrUnavailable,
				path,
			)
		}

		return number, nil
	}

	return 0, nil
}

// Check says whether the system offers what the watch reads, and if not, what
// it could not read. It reads the machine at meminfo, looks for a cgroup v2
// mount at root, and reads this process's cgroup through self.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
func Check(
	meminfo string,
	root string,
	self string,
) error {
	_, err := Machine(meminfo)
	if err != nil {
		return err
	}

	_, err = os.Stat(root + "/" + MARKER)
	if err != nil {
		return fmt.Errorf(
			"%w: no cgroup v2 at %s (cannot read %s): %w",
			ErrUnavailable,
			root,
			MARKER,
			err,
		)
	}

	_, err = Cgroup(
		root,
		self,
	)

	return err
}
