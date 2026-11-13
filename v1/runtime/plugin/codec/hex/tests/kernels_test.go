package tests

import (
	"bytes"
	stdhex "encoding/hex"
	"math/rand/v2"
	"strings"
	"testing"
)

const (
	// GUARD is how many bytes of a known pattern lie each side of a destination,
	// and GUARD_BYTE the pattern. A kernel that writes outside its range changes
	// one.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	GUARD      = 64
	GUARD_BYTE = 0xA5

	// SWEEP is the largest size that the sweeps run at every size. It is past
	// every threshold and past two blocks of the widest kernel.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SWEEP = 600
)

var (
	// REFUSE_SIZES and POSITION_SIZES are digit counts round every block size
	// of the kernels: 16, 32 and 64 digits, and one pair more or less.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	REFUSE_SIZES   = []int{2, 8, 16, 30, 32, 34, 62, 64, 66, 126, 128, 130, 258, 514}
	POSITION_SIZES = []int{16, 32, 48, 64, 66, 96, 128, 130, 192, 258, 514}

	// BAD_BYTES are bytes next to a run of digits, and bytes with bit 7 set.
	//
	// Revisions:
	//   - 2026-11-03 21:39: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BAD_BYTES = []byte{'g', 'G', ' ', 0x80, 0xff, ':', '/'}
)

// _Offsets are the positions of a text of size digits that a sweep checks: the
// first pair, the middle, and the last pair.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Offsets(size int) []int {
	return []int{0, 1, size / 2, size - 2, size - 1}
}

// _Reference is the upper-case hex of data by the standard library: the
// specification that every path must equal.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Reference(data []byte) string {
	return strings.ToUpper(stdhex.EncodeToString(data))
}

// _Random is size bytes from a generator with the seed given.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Random(
	seed uint64,
	size int,
) []byte {
	var (
		rng = rand.New(rand.NewPCG(
			seed,
			seed^0x9e3779b97f4a7c15,
		))
		data = make(
			[]byte,
			size,
		)
	)

	for idx := range data {
		data[idx] = byte(rng.Uint32())
	}

	return data
}

// _Guarded is a buffer of size bytes that has GUARD_BYTE round it, and the
// slice of the middle.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Guarded(size int) ([]byte, []byte) {
	whole := bytes.Repeat(
		[]byte{GUARD_BYTE},
		size+2*GUARD,
	)

	return whole, whole[GUARD : GUARD+size]
}

// _EachLevel runs the body on every path that this CPU has, as a subtest named
// for it, and puts the level back after.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _EachLevel(
	t *testing.T,
	body func(
		t *testing.T,
		level int,
	),
) {
	t.Helper()

	for _, level := range Levels() {
		t.Run(
			LevelName(level),
			func(t *testing.T) {
				t.Helper()

				body(
					t,
					level,
				)
			},
		)
	}
}

// _CheckEncode compares the path of the level in use to the reference for data,
// and watches the bytes round the destination.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _CheckEncode(
	t *testing.T,
	level int,
	data []byte,
) {
	t.Helper()

	whole, dst := _Guarded(len(data) * 2)

	EncodeAt(
		level,
		dst,
		data,
	)

	if want := _Reference(data); string(dst) != want {
		t.Fatalf(
			"size %d: got %q, want %q",
			len(data),
			dst,
			want,
		)
	}

	for idx, octet := range whole[:GUARD] {
		if octet != GUARD_BYTE {
			t.Fatalf(
				"size %d: wrote before dst at %d",
				len(data),
				idx,
			)
		}
	}

	for idx, octet := range whole[GUARD+len(dst):] {
		if octet != GUARD_BYTE {
			t.Fatalf(
				"size %d: wrote after dst at %d",
				len(data),
				idx,
			)
		}
	}
}

// _CheckDecode compares the path of the level in use to encoding/hex for
// digits. Both must agree on whether the digits are valid and, when they are,
// on the bytes.
//
// Revisions:
//   - 2026-11-03 01:13: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _CheckDecode(
	t *testing.T,
	level int,
	digits []byte,
) {
	t.Helper()

	whole, dst := _Guarded(len(digits) / 2)

	var (
		want, err = stdhex.DecodeString(string(digits))
		valid     = DecodeAt(
			level,
			dst,
			digits,
		)
	)

	if valid != (err == nil) {
		t.Fatalf(
			"size %d: valid %v, but encoding/hex gave %v for %q",
			len(digits),
			valid,
			err,
			digits,
		)
	}

	if valid && !bytes.Equal(
		dst,
		want,
	) {
		t.Fatalf(
			"size %d: got %x, want %x",
			len(digits),
			dst,
			want,
		)
	}

	for idx, octet := range whole[:GUARD] {
		if octet != GUARD_BYTE {
			t.Fatalf(
				"size %d: wrote before dst at %d",
				len(digits),
				idx,
			)
		}
	}

	for idx, octet := range whole[GUARD+len(dst):] {
		if octet != GUARD_BYTE {
			t.Fatalf(
				"size %d: wrote after dst at %d",
				len(digits),
				idx,
			)
		}
	}
}

// TestKernels_EncodeMatchesTheReference writes the hex of random bytes of every
// size from 0 to SWEEP, at every level the CPU has, and compares it to the
// standard library's, upper-cased. The destination is watched for a write
// outside its range.
//
// Success is no mismatch, at every size, at every level. A level the CPU lacks
// is not listed, and the log says which ones ran.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_EncodeMatchesTheReference(t *testing.T) {
	t.Logf(
		"levels run: %v, highest %s",
		Levels(),
		LevelName(Detected()),
	)

	_EachLevel(
		t,
		func(
			t *testing.T,
			level int,
		) {
			t.Helper()

			for size := 0; size <= SWEEP; size++ {
				_CheckEncode(
					t,
					level,
					_Random(
						uint64(size),
						size,
					),
				)
			}
		},
	)
}

// TestKernels_EncodeHandlesEveryByteAtEveryOffset puts each of the 256 byte
// values in each lane of a block, so a lane that handles one value wrongly is
// found.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_EncodeHandlesEveryByteAtEveryOffset(t *testing.T) {
	_EachLevel(
		t,
		func(
			t *testing.T,
			level int,
		) {
			t.Helper()

			for _, size := range []int{4, 8, 16, 31, 32, 33, 64, 65, 130} {
				for value := range 256 {
					for offset := range size {
						data := make(
							[]byte,
							size,
						)
						data[offset] = byte(value)

						_CheckEncode(
							t,
							level,
							data,
						)
					}
				}
			}
		},
	)
}

// TestKernels_DecodeMatchesTheReference decodes random digits, in the case
// mix a script can send, of every even size from 0 to SWEEP.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeMatchesTheReference(t *testing.T) {
	_EachLevel(
		t,
		func(
			t *testing.T,
			level int,
		) {
			t.Helper()

			for size := 0; size <= SWEEP; size += 2 {
				var (
					data = _Random(
						uint64(size),
						size/2,
					)
					digits = []byte(_Reference(data))
					rng    = rand.New(rand.NewPCG(
						uint64(size),
						7,
					))
				)

				for idx, char := range digits {
					if char >= 'A' && rng.IntN(2) == 0 {
						digits[idx] = char + ('a' - 'A')
					}
				}

				_CheckDecode(
					t,
					level,
					digits,
				)
			}
		},
	)
}

// TestKernels_DecodeRefusesTheSameBytes puts each of the 256 byte values at the
// first, middle and last digit of texts of sizes round every block size, and
// requires each path to agree with encoding/hex on whether it is valid. The
// values include the neighbours of each run ('/', ':', '@', 'G', '`', 'g'), the
// bytes with bit 7 set, and NUL.
//
// This is the strictness of C22: a byte that is not a digit is refused, at any
// position, by every path.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeRefusesTheSameBytes(t *testing.T) {
	_EachLevel(
		t,
		func(
			t *testing.T,
			level int,
		) {
			t.Helper()

			for _, size := range REFUSE_SIZES {
				for value := range 256 {
					for _, offset := range _Offsets(size) {
						digits := bytes.Repeat(
							[]byte{'7'},
							size,
						)
						digits[offset] = byte(value)

						_CheckDecode(
							t,
							level,
							digits,
						)
					}
				}
			}
		},
	)
}

// TestKernels_DecodeRefusesEveryPosition puts a bad digit at each position of
// texts of several sizes, so a position that a kernel never checks is found:
// the lanes, the block ends, and the overlapped tail.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeRefusesEveryPosition(t *testing.T) {
	_EachLevel(
		t,
		func(
			t *testing.T,
			level int,
		) {
			t.Helper()

			for _, size := range POSITION_SIZES {
				for offset := range size {
					for _, bad := range BAD_BYTES {
						digits := bytes.Repeat(
							[]byte{'a'},
							size,
						)
						digits[offset] = bad

						_CheckDecode(
							t,
							level,
							digits,
						)
					}
				}
			}
		},
	)
}

// TestKernels_EncodeSWAREqualsTheReference runs the portable word path, which is
// reachable at every level, against the reference on every size, so a build
// where the CPU has no kernel still tests it.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_EncodeSWAREqualsTheReference(t *testing.T) {
	for size := 0; size <= SWEEP; size++ {
		var (
			data = _Random(
				uint64(size)+1000,
				size,
			)
			dst = make(
				[]byte,
				size*2,
			)
		)

		EncodeSWAR(
			dst,
			data,
		)

		if string(dst) != _Reference(data) {
			t.Fatalf(
				"encode swar size %d",
				size,
			)
		}
	}
}

// TestKernels_LevelsIncludeTheHighestTheCPUHas pins the detection: the level
// list ends at the level that CPUID reports, and starts at the portable one.
// On a CPU with no AVX-512 the log says so, and that path did not run.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_LevelsIncludeTheHighestTheCPUHas(t *testing.T) {
	levels := Levels()

	if len(levels) == 0 || levels[0] != 0 {
		t.Fatalf(
			"levels %v do not start at the portable level",
			levels,
		)
	}

	if levels[len(levels)-1] != Detected() {
		t.Fatalf(
			"levels %v do not end at the detected level %d",
			levels,
			Detected(),
		)
	}

	t.Logf(
		"highest level: %s",
		LevelName(Detected()),
	)
}

// FuzzKernels_EncodeAgrees fuzzes the bytes and the level: every path must
// write what the reference writes.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzKernels_EncodeAgrees(f *testing.F) {
	for _, size := range []int{0, 1, 15, 16, 31, 32, 33, 63, 64, 65, 200} {
		f.Add(_Random(
			uint64(size),
			size,
		))
	}

	f.Fuzz(
		func(
			t *testing.T,
			data []byte,
		) {
			_EachLevel(
				t,
				func(
					t *testing.T,
					level int,
				) {
					t.Helper()

					_CheckEncode(
						t,
						level,
						data,
					)
				},
			)
		},
	)
}

// FuzzKernels_DecodeAgrees fuzzes the digits: every path must agree with
// encoding/hex on validity and on the bytes. An odd text is made even by one
// more digit, as _Decode does.
//
// Revisions:
//   - 2026-11-03 01:11: initial creation (C50)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzKernels_DecodeAgrees(f *testing.F) {
	for _, size := range []int{0, 2, 16, 32, 34, 64, 66, 128, 130, 200} {
		f.Add([]byte(_Reference(_Random(
			uint64(size),
			size/2,
		))))
	}

	f.Add([]byte(strings.Repeat(
		"aF09",
		40,
	) + "g1"))

	f.Fuzz(
		func(
			t *testing.T,
			digits []byte,
		) {
			if len(digits)%2 != 0 {
				digits = append(
					digits,
					'0',
				)
			}

			_EachLevel(
				t,
				func(
					t *testing.T,
					level int,
				) {
					t.Helper()

					_CheckDecode(
						t,
						level,
						digits,
					)
				},
			)
		},
	)
}
