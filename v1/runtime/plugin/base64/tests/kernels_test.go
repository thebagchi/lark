package tests

import (
	"bytes"
	stdbase64 "encoding/base64"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
)

const (
	// GUARD is how many bytes of a known pattern lie each side of a destination,
	// and GUARD_BYTE the pattern. A kernel that writes outside its range changes
	// one.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	GUARD      = 64
	GUARD_BYTE = 0xA5

	// LONG is the largest size of the whole-range sweep: lengths 0 to LONG.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LONG = 4096

	// LONG_ENV names the environment variable that asks for the full sweep of
	// the large random test.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LONG_ENV = "LARK_LONG"
)

var (
	// ALPHABETS are the two alphabets of the builtins: the standard one, which
	// is written with padding, and the URL one, which is not.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ALPHABETS = []int{spec.ALPHABET_STD, spec.ALPHABET_URL}

	// REFUSE_SIZES are character counts round every block of the kernels: 16,
	// 32 and 64 characters, and a group more or less. Each also puts the bad
	// character in the kernel part or in the tail that encoding/base64 reads.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	REFUSE_SIZES = []int{
		2, 3, 4, 12, 16, 17, 20, 31, 32, 33, 36, 63, 64, 65, 68,
		100, 128, 130, 132, 258, 516,
	}

	// POSITION_SIZES are character counts for the sweep of every position.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	POSITION_SIZES = []int{16, 32, 48, 64, 68, 96, 128, 132, 200, 260}

	// LAST_SIZES are character counts that end in a part group, and so in pad
	// bits: 2 and 3 modulo 4, in the kernel part and in the tail.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LAST_SIZES = []int{2, 3, 34, 35, 66, 67, 130, 131, 258, 259, 1026, 1027}

	// NOT_LINE_BREAK holds every byte value but CR and LF. The builtins refuse a
	// line break before any decoder runs (unpack.CheckPadding), so _DecodeAt is
	// never given one; encoding/base64 would skip it, a kernel would refuse it.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	NOT_LINE_BREAK = _NotLineBreaks()
)

// _NotLineBreaks is every byte value but CR and LF.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _NotLineBreaks() []byte {
	var out []byte

	for value := range 256 {
		if value != '\r' && value != '\n' {
			out = append(
				out,
				byte(value),
			)
		}
	}

	return out
}

// _Rounds is how many random large texts a level reads: 200 when LARK_LONG is
// set, and 12 otherwise.
//
// Revisions:
//   - 2026-11-04 06:44: initial creation (C51 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Rounds() int {
	if os.Getenv(LONG_ENV) != "" {
		return 200
	}

	return 12
}

// _Span is the largest random size above LONG: 1 MiB when LARK_LONG is set,
// and 128 KiB otherwise.
//
// Revisions:
//   - 2026-11-04 06:44: initial creation (C51 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Span() int {
	if os.Getenv(LONG_ENV) != "" {
		return 1 << 20
	}

	return 1 << 17
}

// _Room is the bytes that the bare text of the given size stands for.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Room(size int) int {
	return stdbase64.RawStdEncoding.DecodedLen(size)
}

// _Length is the characters of the encoding of size bytes in the alphabet.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Length(
	alphabet int,
	size int,
) int {
	return _Encoding(alphabet).EncodedLen(size)
}

// _Offsets are the places in a text of the given size where a sweep sets a
// byte: the first two, the middle, and the last two.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Offsets(size int) []int {
	return []int{0, 1, size / 2, size - 2, size - 1}
}

// _Mode is one path under test: a level and an alphabet.
//
// Revisions:
//   - 2026-11-04 19:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Mode struct {
	Level    int
	Alphabet int
}

// _Name is the subtest name of a level and an alphabet.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Name(
	level int,
	alphabet int,
) string {
	if alphabet == spec.ALPHABET_URL {
		return LevelName(level) + "/url"
	}

	return LevelName(level) + "/std"
}

// _Encoding is the encoding that a builtin of the alphabet writes by.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Encoding(alphabet int) *stdbase64.Encoding {
	if alphabet == spec.ALPHABET_URL {
		return stdbase64.RawURLEncoding
	}

	return stdbase64.StdEncoding
}

// _Reference is what the standard library writes: the specification that every
// path must equal.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Reference(
	alphabet int,
	data []byte,
) []byte {
	return []byte(_Encoding(alphabet).EncodeToString(data))
}

// _Bare is the text of data with no padding, in the alphabet.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Bare(
	alphabet int,
	data []byte,
) []byte {
	if alphabet == spec.ALPHABET_URL {
		return []byte(stdbase64.RawURLEncoding.EncodeToString(data))
	}

	return []byte(stdbase64.RawStdEncoding.EncodeToString(data))
}

// _Strict is the standard library's verdict on bare text: the specification of
// what a decoder accepts.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Strict(
	alphabet int,
	text []byte,
) ([]byte, bool) {
	encoding := stdbase64.RawStdEncoding.Strict()
	if alphabet == spec.ALPHABET_URL {
		encoding = stdbase64.RawURLEncoding.Strict()
	}

	data, err := encoding.DecodeString(string(text))

	return data, err == nil
}

// _Random is size bytes from a generator with the seed given.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
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

// _Guarded is a buffer of size bytes between two runs of GUARD_BYTE.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Guarded(size int) ([]byte, []byte) {
	whole := bytes.Repeat(
		[]byte{GUARD_BYTE},
		GUARD+size+GUARD,
	)

	return whole, whole[GUARD : GUARD+size : GUARD+size]
}

// _GuardsKept reports whether both guards still hold the pattern.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _GuardsKept(whole []byte) bool {
	for _, idx := range [][2]int{
		{0, GUARD},
		{len(whole) - GUARD, len(whole)},
	} {
		for _, value := range whole[idx[0]:idx[1]] {
			if value != GUARD_BYTE {
				return false
			}
		}
	}

	return true
}

// TestKernels_EncodeMatchesTheReference proves every level writes what
// encoding/base64 writes, for every size 0 to LONG, in both alphabets, and
// writes nothing outside the destination.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_EncodeMatchesTheReference(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for size := 0; size <= LIMIT; size++ {
						var (
							data = _Random(
								uint64(size),
								size,
							)
							want = _Reference(
								alphabet,
								data,
							)
							whole, dest = _Guarded(len(want))
						)

						EncodeAt(
							level,
							alphabet,
							dest,
							data,
						)

						if !bytes.Equal(
							dest,
							want,
						) {
							t.Fatalf(
								"size %d: wrong text",
								size,
							)
						}

						if !_GuardsKept(whole) {
							t.Fatalf(
								"size %d: wrote outside",
								size,
							)
						}
					}
				},
			)
		}
	}
}

// TestKernels_EncodeHandlesEveryByteAtEveryOffset proves each of the 256 byte
// values is encoded right in each position of a block: one byte is set in zeros
// at a time, and at every offset in two blocks of the widest kernel.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_EncodeHandlesEveryByteAtEveryOffset(t *testing.T) {
	const SIZE = 110

	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					var (
						data = make(
							[]byte,
							SIZE,
						)
						dest = make(
							[]byte,
							_Encoding(alphabet).EncodedLen(SIZE),
						)
					)

					for offset := range SIZE {
						for value := range 256 {
							data[offset] = byte(value)

							EncodeAt(
								level,
								alphabet,
								dest,
								data,
							)

							if !bytes.Equal(
								dest,
								_Reference(
									alphabet,
									data,
								),
							) {
								t.Fatalf(
									"%#x at %d: wrong",
									value,
									offset,
								)
							}
						}

						data[offset] = 0
					}
				},
			)
		}
	}
}

// TestKernels_DecodeMatchesTheReference proves every level reads the bare text
// of random data to the data, and writes nothing outside the destination, for
// every size 0 to LONG.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeMatchesTheReference(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for size := 0; size <= LIMIT; size++ {
						var (
							data = _Random(
								uint64(size),
								size,
							)
							text = _Bare(
								alphabet,
								data,
							)
							whole, dest = _Guarded(_Room(len(text)))
						)

						count, ok := DecodeAt(
							level,
							alphabet,
							dest,
							text,
						)

						if !ok || !bytes.Equal(
							dest[:count],
							data,
						) {
							t.Fatalf(
								"size %d: wrong bytes or refused",
								size,
							)
						}

						if !_GuardsKept(whole) {
							t.Fatalf(
								"size %d: wrote outside",
								size,
							)
						}
					}
				},
			)
		}
	}
}

// TestKernels_DecodeRefusesTheSameText proves each of the 254 byte values
// that are not CR or LF, set at the first, second, middle, last and last but
// one place of texts round the block sizes, gives the verdict of
// encoding/base64 in Strict mode, with the same bytes when it accepts.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeRefusesTheSameText(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for _, size := range REFUSE_SIZES {
						text := _Bare(
							alphabet,
							_Random(
								uint64(size),
								size*3/4+1,
							),
						)[:size]

						for _, offset := range _Offsets(size) {
							_CheckOffset(
								t,
								&_Mode{
									level,
									alphabet,
								},
								text,
								offset,
							)
						}
					}
				},
			)
		}
	}
}

// _CheckOffset sets each byte value at offset in text, and compares the verdict
// and the bytes of the path with the standard library's.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _CheckOffset(
	t *testing.T,
	mode *_Mode,
	text []byte,
	offset int,
) {
	t.Helper()

	var (
		kept = text[offset]
		dest = make(
			[]byte,
			_Room(len(text)),
		)
	)

	defer func() {
		text[offset] = kept
	}()

	for _, value := range NOT_LINE_BREAK {
		text[offset] = value

		var (
			want, wantOK = _Strict(
				mode.Alphabet,
				text,
			)
			count, ok = DecodeAt(
				mode.Level,
				mode.Alphabet,
				dest,
				text,
			)
		)

		if ok != wantOK || ok && !bytes.Equal(
			dest[:count],
			want,
		) {
			t.Fatalf(
				"size %d offset %d value %#x: got %v, reference %v",
				len(text),
				offset,
				value,
				ok,
				wantOK,
			)
		}
	}
}

// TestKernels_DecodeRefusesEveryPosition proves a character outside the
// alphabet is refused at every position of texts round the block sizes.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeRefusesEveryPosition(t *testing.T) {
	var bad = []byte{'=', ' ', '.', '*', 0x00, 0x80, 0xff, '@', '[', '`', '{', ':'}

	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for _, size := range POSITION_SIZES {
						var (
							text = _Bare(
								alphabet,
								_Random(
									uint64(size),
									size*3/4+1,
								),
							)[:size]
							dest = make(
								[]byte,
								_Room(size),
							)
						)

						for position := range size {
							var kept = text[position]

							for _, value := range bad {
								text[position] = value

								if _, ok := DecodeAt(
									level,
									alphabet,
									dest,
									text,
								); ok {
									t.Fatalf(
										"taken: %d %d %#x",
										size,
										position,
										value,
									)
								}
							}

							text[position] = kept
						}
					}
				},
			)
		}
	}
}

// TestKernels_DecodeStrictLastGroup proves the pad bits of the last group are
// still checked when a kernel read the rest of a long text (C22): the last
// character takes every value of the alphabet, at sizes 2 and 3 modulo 4.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeStrictLastGroup(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for _, size := range LAST_SIZES {
						text := _Bare(
							alphabet,
							_Random(
								uint64(size),
								size*3/4+1,
							),
						)[:size]

						_CheckOffset(
							t,
							&_Mode{
								level,
								alphabet,
							},
							text,
							size-1,
						)
					}
				},
			)
		}
	}
}

// TestKernels_DecodeLargeRandom proves each level reads random large texts to
// the data and refuses the same texts with one bad character. A normal run does
// 12 texts up to 128 KiB, which stays well inside the default timeout under
// -race; set LARK_LONG=1 for the full sweep of 200 texts up to 1 MiB.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeLargeRandom(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					rng := rand.New(rand.NewPCG(
						7,
						11,
					))

					for round := range _Rounds() {
						size := rng.IntN(_Span()) + LONG + 1

						var (
							data = _Random(
								uint64(round),
								size,
							)
							text = _Bare(
								alphabet,
								data,
							)
							dest = make(
								[]byte,
								_Room(len(text)),
							)
							out = make(
								[]byte,
								_Length(
									alphabet,
									size,
								),
							)
						)

						EncodeAt(
							level,
							alphabet,
							out,
							data,
						)

						if !bytes.Equal(
							out,
							_Reference(
								alphabet,
								data,
							),
						) {
							t.Fatalf(
								"round %d: wrong text",
								round,
							)
						}

						count, ok := DecodeAt(
							level,
							alphabet,
							dest,
							text,
						)
						if !ok || !bytes.Equal(
							dest[:count],
							data,
						) {
							t.Fatalf(
								"round %d: wrong bytes",
								round,
							)
						}

						text[rng.IntN(len(text))] = '*'

						if _, ok := DecodeAt(
							level,
							alphabet,
							dest,
							text,
						); ok {
							t.Fatalf(
								"round %d: took a bad byte",
								round,
							)
						}
					}
				},
			)
		}
	}
}

// TestKernels_LevelsIncludeTheHighestTheCPUHas pins the detection: the level
// list starts at the standard library alone and ends at the detected level.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_LevelsIncludeTheHighestTheCPUHas(t *testing.T) {
	levels := Levels()

	if levels[0] != 0 {
		t.Fatalf(
			"first level %d, want 0",
			levels[0],
		)
	}

	if levels[len(levels)-1] != Detected() {
		t.Fatalf(
			"last level %d, detected %d",
			levels[len(levels)-1],
			Detected(),
		)
	}

	t.Logf(
		"levels run: %v, highest %s",
		levels,
		LevelName(Detected()),
	)
}

// FuzzKernels_EncodeAgrees fuzzes the encoders against the standard library.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzKernels_EncodeAgrees(f *testing.F) {
	f.Add([]byte("hello, world"))
	f.Add(_Random(
		1,
		300,
	))

	f.Fuzz(func(
		t *testing.T,
		data []byte,
	) {
		for _, level := range Levels() {
			for _, alphabet := range ALPHABETS {
				dest := make(
					[]byte,
					_Encoding(alphabet).EncodedLen(len(data)),
				)

				EncodeAt(
					level,
					alphabet,
					dest,
					data,
				)

				if !bytes.Equal(
					dest,
					_Reference(
						alphabet,
						data,
					),
				) {
					t.Fatalf(
						"level %d alphabet %v: wrong text",
						level,
						alphabet,
					)
				}
			}
		}
	})
}

// FuzzKernels_DecodeAgrees fuzzes the decoders against the standard library in
// Strict mode: the same verdict, and the same bytes when it accepts.
//
// Revisions:
//   - 2026-11-03 22:55: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzKernels_DecodeAgrees(f *testing.F) {
	f.Add(_Bare(
		spec.ALPHABET_STD,
		_Random(
			2,
			200,
		),
	))
	f.Add([]byte("Zm9v*mFy"))

	f.Fuzz(func(
		t *testing.T,
		text []byte,
	) {
		if bytes.ContainsAny(
			text,
			"\r\n",
		) {
			return
		}

		for _, level := range Levels() {
			for _, alphabet := range ALPHABETS {
				var (
					want, wantOK = _Strict(
						alphabet,
						text,
					)
					dest = make(
						[]byte,
						_Room(len(text)),
					)
					count, ok = DecodeAt(
						level,
						alphabet,
						dest,
						text,
					)
				)

				if ok != wantOK || ok && !bytes.Equal(
					dest[:count],
					want,
				) {
					t.Fatalf(
						"level %d alphabet %v: got %v, reference %v",
						level,
						alphabet,
						ok,
						wantOK,
					)
				}
			}
		}
	})
}

// TestKernels_ShippedSizesMatchTheReference is the sweep of the first two tests
// with the sizes that ship, which are the path a script reaches: every level,
// both alphabets, every size 0 to LONG, encode and decode.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_ShippedSizesMatchTheReference(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					for size := 0; size <= LIMIT; size++ {
						var (
							data = _Random(
								uint64(size),
								size,
							)
							want = _Reference(
								alphabet,
								data,
							)
							dest = make(
								[]byte,
								len(want),
							)
						)

						EncodeShipped(
							level,
							alphabet,
							dest,
							data,
						)

						if !bytes.Equal(
							dest,
							want,
						) {
							t.Fatalf(
								"encode size %d: wrong text",
								size,
							)
						}

						var (
							text = _Bare(
								alphabet,
								data,
							)
							out = make(
								[]byte,
								_Room(len(text)),
							)
							count, ok = DecodeShipped(
								level,
								alphabet,
								out,
								text,
							)
						)
						if !ok || !bytes.Equal(
							out[:count],
							data,
						) {
							t.Fatalf(
								"decode size %d: wrong or refused",
								size,
							)
						}
					}
				},
			)
		}
	}
}

// TestKernels_DecodeRefusesOneCharacterIntoNoRoom proves a text of one
// character, which no decoder accepts and which stands for no byte, is refused
// at every level without a kernel touching the empty destination. With the
// smallest sizes the kernel path is entered for one character, and its
// empty-destination return is the path under test.
//
// Revisions:
//   - 2026-11-04 06:44: initial creation (C51 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_DecodeRefusesOneCharacterIntoNoRoom(t *testing.T) {
	for _, level := range Levels() {
		for _, alphabet := range ALPHABETS {
			t.Run(
				_Name(
					level,
					alphabet,
				),
				func(t *testing.T) {
					_, ok := DecodeAt(
						level,
						alphabet,
						nil,
						[]byte("A"),
					)
					if ok {
						t.Fatalf("one character was accepted")
					}
				},
			)
		}
	}
}
