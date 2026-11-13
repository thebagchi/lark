package tests

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch/internal/watch"
)

const (
	// MIB and GIB are the sizes the room tests are written in.
	//
	// Revisions:
	//   - 2026-11-03 22:33: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	MIB = 1 << 20
	GIB = 1 << 30
)

// TestFit proves the rule of Fit: a size below the skip size is never read,
// a size above the free memory is refused, and a size that fits passes. Free
// memory is Total less Used of the tighter reading.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestFit(t *testing.T) {
	tests := map[string]struct {
		size    uint64
		used    uint64
		total   uint64
		reads   int32
		refused bool
	}{
		"below the skip size is never read": {
			size:    common.ROOM_SKIP - 1,
			used:    100,
			total:   100,
			reads:   0,
			refused: false,
		},
		"the skip size is read": {
			size:    common.ROOM_SKIP,
			used:    0,
			total:   GIB,
			reads:   1,
			refused: false,
		},
		"fits with room to spare": {
			size:    GIB,
			used:    2 * GIB,
			total:   8 * GIB,
			reads:   1,
			refused: false,
		},
		"exactly the free memory fits": {
			size:    6 * GIB,
			used:    2 * GIB,
			total:   8 * GIB,
			reads:   1,
			refused: false,
		},
		"one byte over the free memory is refused": {
			size:    6*GIB + 1,
			used:    2 * GIB,
			total:   8 * GIB,
			reads:   1,
			refused: true,
		},
		"more than the machine is refused": {
			size:    1 << 62,
			used:    0,
			total:   8 * GIB,
			reads:   1,
			refused: true,
		},
		"use above the total leaves no room": {
			size:    MIB,
			used:    9 * GIB,
			total:   8 * GIB,
			reads:   1,
			refused: true,
		},
	}

	for name, test := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				var reads atomic.Int32

				err := watch.Fit(
					"who",
					test.size,
					func() (*common.MemoryReading, error) {
						reads.Add(1)

						return &common.MemoryReading{
							Used:  test.used,
							Total: test.total,
						}, nil
					},
				)
				if got := errors.Is(
					err,
					watch.ErrRoom,
				); got != test.refused {
					t.Fatalf(
						"refused = %v (%v), want %v",
						got,
						err,
						test.refused,
					)
				}

				if !test.refused && err != nil {
					t.Fatalf(
						"err = %v, want nil",
						err,
					)
				}

				if reads.Load() != test.reads {
					t.Fatalf(
						"reads = %d, want %d",
						reads.Load(),
						test.reads,
					)
				}
			},
		)
	}
}

// TestFit_Unreadable proves Q56: where the reading fails, a size up to 1 GiB
// passes and a size above it is refused. The read error stays out of the
// message.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestFit_Unreadable(t *testing.T) {
	var (
		unreadable = func() (*common.MemoryReading, error) {
			return nil, fmt.Errorf(
				"%w: no /proc/meminfo",
				watch.ErrUnavailable,
			)
		}
		tests = map[string]struct {
			size    uint64
			refused bool
		}{
			"below the skip size": {
				size:    1,
				refused: false,
			},
			"the skip size": {
				size:    common.ROOM_SKIP,
				refused: false,
			},
			"half a gibibyte": {
				size:    GIB / 2,
				refused: false,
			},
			"exactly one gibibyte": {
				size:    GIB,
				refused: false,
			},
			"one byte over a gibibyte": {
				size:    GIB + 1,
				refused: true,
			},
			"eight gibibytes": {
				size:    8 * GIB,
				refused: true,
			},
		}
	)

	for name, test := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				err := watch.Fit(
					"who",
					test.size,
					unreadable,
				)
				if got := errors.Is(
					err,
					watch.ErrRoom,
				); got != test.refused {
					t.Fatalf(
						"refused = %v (%v), want %v",
						got,
						err,
						test.refused,
					)
				}

				if err != nil && strings.Contains(
					err.Error(),
					"meminfo",
				) {
					t.Fatalf(
						"message holds the read error: %v",
						err,
					)
				}
			},
		)
	}
}

// TestFit_Message proves the text: the name, the size in bytes, then the
// sentinel, as random.bytes words its range error.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestFit_Message(t *testing.T) {
	const WANT = "hex.from_int: 8589934592 bytes: not enough free memory"

	err := watch.Fit(
		"hex.from_int",
		8*GIB,
		func() (*common.MemoryReading, error) {
			return &common.MemoryReading{
				Used:  0,
				Total: GIB,
			}, nil
		},
	)

	if err == nil || err.Error() != WANT {
		t.Fatalf(
			"err = %v, want %q",
			err,
			WANT,
		)
	}
}

// TestRoom_UsesTheSystem proves Room reads the real machine: a size no
// machine holds is refused (by the reading, or by the ceiling where the
// system has none), and a size under the skip size passes.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestRoom_UsesTheSystem(t *testing.T) {
	err := watch.Room(
		"who",
		1<<62,
	)
	if !errors.Is(
		err,
		watch.ErrRoom,
	) {
		t.Fatalf(
			"1<<62 bytes: err = %v, want ErrRoom",
			err,
		)
	}

	err = watch.Room(
		"who",
		common.ROOM_SKIP-1,
	)
	if err != nil {
		t.Fatalf(
			"small size: err = %v, want nil",
			err,
		)
	}
}

// BenchmarkSystem measures one reading of the machine and the cgroup, the
// price Fit pays above the skip size.
//
// Run: go test -run '^$' -bench 'System|Allocation' -benchmem -count 6
// ./v1/runtime/memwatch/internal/watch/
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkSystem(b *testing.B) {
	read := watch.System()

	b.ReportAllocs()

	for b.Loop() {
		_, err := read()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAllocation measures making and filling a buffer of each size, the
// work a guarded call does after the check. Compare with BenchmarkSystem.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkAllocation(b *testing.B) {
	for _, size := range []int{
		4 << 10,
		64 << 10,
		256 << 10,
		MIB,
		4 * MIB,
	} {
		b.Run(
			fmt.Sprintf(
				"%dKiB",
				size>>10,
			),
			func(b *testing.B) {
				b.ReportAllocs()

				for b.Loop() {
					buffer := make(
						[]byte,
						size,
					)
					for idx := range buffer {
						buffer[idx] = '0'
					}

					_ = buffer
				}
			},
		)
	}
}

// TestBounded proves the free memory is the smallest free byte count of the
// machine, the cgroup and the address-space limit, not the source with the
// larger share in use. The fake machine reads 1000 kB total and 200 kB
// available: 204,800 bytes free, 80 percent in use.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestBounded(t *testing.T) {
	const MACHINE_FREE = 200 * 1024

	tests := map[string]struct {
		cgroup  map[string]string
		address *common.MemoryReading
		free    uint64
	}{
		"cgroup tighter than the machine": {
			cgroup: map[string]string{
				"memory.max":     "1000\n",
				"memory.current": "900\n",
				"memory.stat":    "inactive_file 0\n",
			},
			free: 100,
		},
		"cgroup with the smaller share but fewer free bytes": {
			cgroup: map[string]string{
				"memory.max":     "100000\n",
				"memory.current": "70000\n",
				"memory.stat":    "inactive_file 0\n",
			},
			free: 30000,
		},
		"machine tighter than the cgroup": {
			cgroup: map[string]string{
				"memory.max":     "1000000000\n",
				"memory.current": "10\n",
				"memory.stat":    "inactive_file 0\n",
			},
			free: MACHINE_FREE,
		},
		"equal": {
			cgroup: map[string]string{
				"memory.max":     "1024000\n",
				"memory.current": "819200\n",
				"memory.stat":    "inactive_file 0\n",
			},
			free: MACHINE_FREE,
		},
		"cgroup without a limit": {
			cgroup: map[string]string{"memory.max": "max\n"},
			free:   MACHINE_FREE,
		},
		"address space tighter than both": {
			cgroup: map[string]string{"memory.max": "max\n"},
			address: &common.MemoryReading{
				Used:  900,
				Total: 1000,
			},
			free: 100,
		},
		"address space looser than the machine": {
			cgroup: map[string]string{"memory.max": "max\n"},
			address: &common.MemoryReading{
				Used:  0,
				Total: 1 << 40,
			},
			free: MACHINE_FREE,
		},
		"address space used above its limit leaves nothing": {
			cgroup: map[string]string{"memory.max": "max\n"},
			address: &common.MemoryReading{
				Used:  2000,
				Total: 1000,
			},
			free: 0,
		},
	}

	for name, test := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					meminfo, root, self = _System(
						t,
						test.cgroup,
					)
					got, err = watch.Bounded(
						meminfo,
						root,
						self,
						test.address,
					)
				)
				if err != nil {
					t.Fatal(err)
				}

				free := got.Total - min(
					got.Used,
					got.Total,
				)
				if free != test.free {
					t.Fatalf(
						"free = %d (%+v), want %d",
						free,
						got,
						test.free,
					)
				}
			},
		)
	}
}

// TestMapped proves the address-space reading: the limit is the total and
// VmSize of the status file is the use. A status file with no VmSize, a bad
// VmSize and a missing file are no reading.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMapped(t *testing.T) {
	var (
		dir   = t.TempDir()
		tests = map[string]struct {
			limit  uint64
			status string
			want   *common.MemoryReading
		}{
			"a limit": {
				limit:  4000000 * 1024,
				status: "Name:\tx\nVmSize:\t  1000 kB\nVmRSS:\t5 kB\n",
				want: &common.MemoryReading{
					Used:  1000 * 1024,
					Total: 4000000 * 1024,
				},
			},
			"no VmSize line": {
				limit:  1 << 30,
				status: "Name:\tx\n",
			},
			"a VmSize that is not a number": {
				limit:  1 << 30,
				status: "VmSize: lots kB\n",
			},
		}
	)

	for name, test := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				got := watch.Mapped(
					test.limit,
					_Write(
						t,
						dir,
						strings.ReplaceAll(
							name,
							" ",
							"_",
						),
						test.status,
					),
				)
				if (got == nil) != (test.want == nil) ||
					(got != nil && *got != *test.want) {
					t.Fatalf(
						"got %+v, want %+v",
						got,
						test.want,
					)
				}
			},
		)
	}

	got := watch.Mapped(
		1<<30,
		dir+"/missing",
	)
	if got != nil {
		t.Fatalf(
			"missing file: got %+v, want nil",
			got,
		)
	}
}

// TestAddress_ReadsTheRealLimit proves Address agrees with this process: with
// no limit it is nil, and with one it holds the limit as Total.
//
// Revisions:
//   - 2026-11-03 21:45: initial creation (C46 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 18:22: reads the limit with _Ceiling, none where there is no RLIMIT_AS
//     (C128, B280, B283)
func TestAddress_ReadsTheRealLimit(t *testing.T) {
	var (
		cur, limited = _Ceiling(t)
		got          = watch.Address()
	)

	if !limited {
		if got != nil {
			t.Fatalf(
				"no limit: got %+v, want nil",
				got,
			)
		}

		return
	}

	if got == nil || got.Total != cur {
		t.Fatalf(
			"got %+v, want Total %d",
			got,
			cur,
		)
	}
}
