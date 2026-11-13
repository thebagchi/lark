package watch

import (
	"fmt"
	"os"
	"strings"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// Room refuses a script-given size of bytes that does not fit the free memory,
// before the caller allocates. Free memory is the smallest free byte count of
// the machine, this process's cgroup and its address-space limit (Free). who
// names the builtin. It works whether or not the memory watch is on. Free()
// captures nothing, so a call below common.ROOM_SKIP allocates nothing.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-03 21:45: reads Free, the least free of three sources (C46 fix round 1)
func Room(
	who string,
	size uint64,
) error {
	return Fit(
		who,
		size,
		Free(),
	)
}

// Free returns the source Room reads: the reading with the fewest free bytes
// among the machine, this process's cgroup and its address-space limit.
//
// System, which the memory watch reads, picks the larger share in use. A size
// check needs the smaller free byte count, and the two differ: a 1 GiB cgroup
// at 70 percent beside a 16 GiB machine at 80 percent has 0.3 GiB free against
// 3.2 GiB, yet the machine has the larger share.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func Free() func() (*common.MemoryReading, error) {
	return func() (*common.MemoryReading, error) {
		return Bounded(
			MEMINFO,
			CGROUP,
			SELF,
			Address(),
		)
	}
}

// Bounded reads the machine at meminfo and the cgroup at root and self, and
// returns the reading with the fewest free bytes (Total less Used) among them
// and address. A nil address, and a cgroup that cannot be read or has no limit,
// leave the others. Only the machine's reading failing is an error. On a tie
// the machine's reading stays.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func Bounded(
	meminfo string,
	root string,
	self string,
	address *common.MemoryReading,
) (*common.MemoryReading, error) {
	least, err := Machine(meminfo)
	if err != nil {
		return nil, err
	}

	limited, err := Cgroup(
		root,
		self,
	)
	if err == nil && limited != nil {
		least = _Fewer(
			least,
			limited,
		)
	}

	if address != nil {
		least = _Fewer(
			least,
			address,
		)
	}

	return least, nil
}

// _Fewer returns the reading with fewer free bytes; a on a tie.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func _Fewer(
	a *common.MemoryReading,
	b *common.MemoryReading,
) *common.MemoryReading {
	if _Spare(b) < _Spare(a) {
		return b
	}

	return a
}

// _Spare is Total less Used, never below zero.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func _Spare(reading *common.MemoryReading) uint64 {
	return reading.Total - min(
		reading.Used,
		reading.Total,
	)
}

// Address reads the address-space limit of this process (ulimit -v) as a
// reading: the limit is the Total and VmSize of /proc/self/status, the address
// space already mapped, is the Used. It is nil when there is no limit, and when
// the mapped size cannot be read.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func Address() *common.MemoryReading {
	limit, found := _Limit()
	if !found {
		return nil
	}

	return Mapped(
		limit,
		STATUS,
	)
}

// Mapped is the reading of an address-space limit of limit bytes against the
// VmSize line of the status file at path. A missing file, a missing line and a
// malformed line give nil: with no way to tell how much is mapped, the limit
// alone would refuse a size the process may hold, so it is not used.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
func Mapped(
	limit uint64,
	path string,
) *common.MemoryReading {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	for line := range strings.SplitSeq(
		string(data),
		"\n",
	) {
		name, rest, found := strings.Cut(
			line,
			":",
		)
		if !found || name != "VmSize" {
			continue
		}

		used, ok := _Kilobytes(rest)
		if !ok {
			return nil
		}

		return &common.MemoryReading{
			Used:  used,
			Total: limit,
		}
	}

	return nil
}

// _Read calls read. It answers ErrUnavailable, a failed read, for a
// nil read or a nil reading.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _Read(
	read func() (*common.MemoryReading, error),
) (*common.MemoryReading, error) {
	if read == nil {
		return nil, ErrUnavailable
	}

	reading, err := read()
	if err == nil && reading == nil {
		return nil, ErrUnavailable
	}

	return reading, err
}

// Fit is Room with the reading given, so a test can inject one.
//
// The rule, in order:
//  1. A size below common.ROOM_SKIP passes and read is not called. The read
//     costs about what making a buffer of that size costs.
//  2. One read per call. When it fails, the memory cannot be known, and a size
//     of common.ROOM_CEILING (1 GiB) or less passes while a larger one is
//     refused (Q56).
//  3. When it works, a size above Total less Used, the free memory of the
//     tighter source, is refused. A size that equals the free memory passes.
//
// The message holds the name and the size, both the caller's own: no input
// text and no read error, so nothing from outside reaches it.
//
// A nil read, and a read that answers no reading and no error, are a read that
// failed: rule 2 applies.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-04 19:30: a nil read or a nil reading is a failed read (C87)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Fit(
	who string,
	size uint64,
	read func() (*common.MemoryReading, error),
) error {
	if size < common.ROOM_SKIP {
		return nil
	}

	var (
		free         = uint64(common.ROOM_CEILING)
		reading, err = _Read(read)
	)
	if err == nil {
		free = reading.Total - min(
			reading.Used,
			reading.Total,
		)
	}

	if size <= free {
		return nil
	}

	return fmt.Errorf(
		"%s: %d bytes: %w",
		who,
		size,
		ErrRoom,
	)
}
