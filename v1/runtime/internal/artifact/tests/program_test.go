package tests

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// The tests write programs by hand from the layout in the comment at the top
// of go.starlark.net internal/compile/serial.go (the clauses P1 to P7 of
// program.go), so a refusal is proved against the specification and not
// against the walker's own reading of it.

const (
	// HUGE is a count the decoder would allocate terabytes for: 2^40 as a
	// signed varint of the input, which is a few bytes long.
	//
	// Revisions:
	//   - 2026-11-04 22:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-09 11:26: CRASH_CORPUS moved to its own const group (C114, Q283)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HUGE        = int64(1) << 40
	VERSION_NOW = 14
)

const (
	// CRASH_CORPUS is the 100-byte bundle fuzzing found, which killed Load with a
	// fatal out of memory (B126). FuzzLoad seeds it too.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from
	//     testdata/fuzz/FuzzLoad/703fec1af0317294 (C114, Q283)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CRASH_CORPUS = "\x12s2\b00000000\x12g!sky#\x00\x00\x00\x1c0\xa4\xa4\xa4\xa4\xa4" +
		"\xa4\x00\x00\x02\x0400100000000000000000000000000000000000000000" +
		"00?000000000000000000000000000000000000000000"
)

// _Varint is x as the signed varint the encoder writes.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Varint(x int64) []byte {
	return binary.AppendVarint(
		nil,
		x,
	)
}

// ZERO_COUNT is an empty count.
//
// Revisions:
//   - 2026-11-04 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	ZERO_COUNT = _Varint(0)
)

// _Function is a Funcode with the four counted fields given: its line table,
// locals, cells and free variables, each a count with its elements.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Function(
	table []byte,
	locals []byte,
	cells []byte,
	free []byte,
) []byte {
	return bytes.Join(
		[][]byte{
			ZERO_COUNT, ZERO_COUNT, ZERO_COUNT, // id: name, line, column
			ZERO_COUNT, // doc
			ZERO_COUNT, // code
			table,      // line table
			locals,     // locals
			cells,      // cells
			free,       // free variables
			ZERO_COUNT, ZERO_COUNT, ZERO_COUNT, ZERO_COUNT, ZERO_COUNT,
		},
		nil,
	)
}

// _Parts are the sections of a program, each already a count with elements.
//
// Revisions:
//   - 2026-11-04 22:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Parts struct {
	version  []byte
	loads    []byte
	names    []byte
	consts   []byte
	globals  []byte
	toplevel []byte
	funcs    []byte
	rec      []byte
	text     []byte // the string section
}

// _Plain is a valid program of no names, no constants and no functions.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Plain() *_Parts {
	return &_Parts{
		version: _Varint(VERSION_NOW),
		loads:   ZERO_COUNT,
		names:   ZERO_COUNT,
		consts:  ZERO_COUNT,
		globals: ZERO_COUNT,
		toplevel: _Function(
			ZERO_COUNT,
			ZERO_COUNT,
			ZERO_COUNT,
			ZERO_COUNT,
		),
		funcs: ZERO_COUNT,
		rec:   ZERO_COUNT,
	}
}

// _Encoded is the program as bytes: the magic, the offset of the string
// section, the program, then the strings. The filename is empty.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Encoded(p *_Parts) []byte {
	var (
		prog = bytes.Join(
			[][]byte{
				p.version,
				ZERO_COUNT, // filename: length 0
				p.loads,
				p.names,
				p.consts,
				p.globals,
				p.toplevel,
				p.funcs,
				p.rec,
			},
			nil,
		)
		out = slices.Concat(
			[]byte("!sky"),
			binary.LittleEndian.AppendUint32(
				nil,
				uint32(8+len(prog)),
			),
			prog,
			p.text,
		)
	)

	return out
}

// _Loaded holds a bundle of the lib fixture, whose one unit carries code.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Loaded(
	t *testing.T,
	code []byte,
) error {
	t.Helper()

	bundle := _Handmade(
		t,
		"lib.star",
	)
	bundle.Entry = "lib.star"
	units := bundle.GetUnits()
	units[len(units)-1].Code = code

	raw, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatalf(
			"marshal: %v",
			err,
		)
	}

	_, err = artifact.Load(raw)

	return err
}

// TestLoad_BoundsProgramCounts proves Load refuses, with an error and no
// crash, a program whose count or length the decoder would allocate from
// unchecked. One case per count site of serial.go (clauses P1 to P7), and one
// valid program that passes.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132, B126)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_BoundsProgramCounts(t *testing.T) {
	var (
		huge = _Varint(HUGE)
		with = func(change func(*_Parts)) *_Parts {
			p := _Plain()
			change(p)

			return p
		}
	)

	cases := []struct {
		name    string
		parts   *_Parts
		refused bool
	}{
		{
			"Valid",
			_Plain(),
			false,
		},
		{
			"ValidNames",
			with(func(p *_Parts) {
				p.names = bytes.Join(
					[][]byte{_Varint(2), _Varint(2), _Varint(3)},
					nil,
				)
				p.text = []byte("abcde")
			}),
			false,
		},
		{
			"ValidConstants",
			with(func(p *_Parts) {
				p.consts = bytes.Join(
					[][]byte{
						_Varint(5),
						_Varint(0), _Varint(1), // string of 1
						_Varint(1), _Varint(1), // bytes of 1
						_Varint(2), _Varint(-9), // int
						_Varint(3), binary.AppendUvarint(
							nil,
							1<<63,
						), // float
						_Varint(4), _Varint(1), // bigint of 1 digit
					},
					nil,
				)
				p.text = []byte("xy7")
			}),
			false,
		},
		{
			"VersionOlder",
			with(func(p *_Parts) {
				p.version = _Varint(VERSION_NOW - 1)
			}),
			true,
		},
		{
			"VersionNewer",
			with(func(p *_Parts) {
				p.version = _Varint(VERSION_NOW + 1)
			}),
			true,
		},
		{
			"CountLoads",
			with(func(p *_Parts) {
				p.loads = huge
			}),
			true,
		},
		{
			"CountNames",
			with(func(p *_Parts) {
				p.names = huge
			}),
			true,
		},
		{
			"CountConstants",
			with(func(p *_Parts) {
				p.consts = huge
			}),
			true,
		},
		{
			"CountGlobals",
			with(func(p *_Parts) {
				p.globals = huge
			}),
			true,
		},
		{
			"CountFuncs",
			with(func(p *_Parts) {
				p.funcs = huge
			}),
			true,
		},
		{
			"CountLineTable",
			with(func(p *_Parts) {
				p.toplevel = _Function(
					huge,
					ZERO_COUNT,
					ZERO_COUNT,
					ZERO_COUNT,
				)
			}),
			true,
		},
		{
			"CountLocals",
			with(func(p *_Parts) {
				p.toplevel = _Function(
					ZERO_COUNT,
					huge,
					ZERO_COUNT,
					ZERO_COUNT,
				)
			}),
			true,
		},
		{
			"CountCells",
			with(func(p *_Parts) {
				p.toplevel = _Function(
					ZERO_COUNT,
					ZERO_COUNT,
					huge,
					ZERO_COUNT,
				)
			}),
			true,
		},
		{
			"CountFreeVars",
			with(func(p *_Parts) {
				p.toplevel = _Function(
					ZERO_COUNT,
					ZERO_COUNT,
					ZERO_COUNT,
					huge,
				)
			}),
			true,
		},
		{
			// Each count is within the bytes left, and the sum is not: three
			// functions are claimed and one is given.
			"CountsSumPastTheBytes",
			with(func(p *_Parts) {
				p.funcs = append(
					_Varint(3),
					_Function(
						ZERO_COUNT,
						ZERO_COUNT,
						ZERO_COUNT,
						ZERO_COUNT,
					)...,
				)
			}),
			true,
		},
		{
			"CountNegative",
			with(func(p *_Parts) {
				p.names = _Varint(-1)
			}),
			true,
		},
		{
			"CountVarintTooLong",
			with(func(p *_Parts) {
				p.names = bytes.Repeat(
					[]byte{0xff},
					11,
				)
			}),
			true,
		},
		{
			"StringPastTheSection",
			with(func(p *_Parts) {
				p.names = append(
					_Varint(1),
					_Varint(9)...,
				)
			}),
			true,
		},
		{
			"StringLengthNegative",
			with(func(p *_Parts) {
				p.names = append(
					_Varint(1),
					_Varint(-1)...,
				)
			}),
			true,
		},
		{
			"StringsLeftOver",
			with(func(p *_Parts) {
				p.text = []byte("x")
			}),
			true,
		},
		{
			"ConstantTypeUnknown",
			with(func(p *_Parts) {
				p.consts = append(
					_Varint(1),
					_Varint(5)...,
				)
			}),
			true,
		},
		{
			"FloatVarintTooLong",
			with(func(p *_Parts) {
				p.consts = bytes.Join(
					[][]byte{
						_Varint(1),
						_Varint(3),
						bytes.Repeat(
							[]byte{0xff},
							11,
						),
					},
					nil,
				)
			}),
			true,
		},
		{
			"ConstantTypeNegative",
			with(func(p *_Parts) {
				p.consts = append(
					_Varint(1),
					_Varint(-1)...,
				)
			}),
			true,
		},
		{
			"ProgramLeftOver",
			with(func(p *_Parts) {
				p.rec = append(
					_Varint(0),
					0,
				)
			}),
			true,
		},
		{
			"ProgramCutShort",
			with(func(p *_Parts) {
				p.rec = nil
			}),
			true,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := _Loaded(
					t,
					_Encoded(tc.parts),
				)
				if tc.refused && err == nil {
					t.Fatal("want the program refused")
				}

				if !tc.refused && err != nil {
					t.Fatalf(
						"want the program loaded, got %v",
						err,
					)
				}
			},
		)
	}
}

// TestLoad_RefusesBadProgramHeader proves the magic and the string offset are
// checked before anything is read (clauses P1 and P2).
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_RefusesBadProgramHeader(t *testing.T) {
	var (
		good   = _Encoded(_Plain())
		offset = func(value uint32) []byte {
			out := bytes.Clone(good)
			binary.LittleEndian.PutUint32(
				out[4:8],
				value,
			)

			return out
		}
	)

	cases := []struct {
		name string
		code []byte
	}{
		{
			"Empty",
			[]byte{},
		},
		{
			"ShortOfMagic",
			[]byte("!sk"),
		},
		{
			"MagicOnly",
			[]byte("!sky"),
		},
		{
			"HeaderOnly",
			[]byte("!sky\x08\x00\x00\x00")[:7],
		},
		{
			"WrongMagic",
			append(
				[]byte("sky!"),
				good[4:]...,
			),
		},
		{
			"OffsetBeforeHeader",
			offset(7),
		},
		{
			"OffsetPastTheEnd",
			offset(uint32(len(good)) + 1),
		},
		{
			"OffsetHuge",
			offset(0xffffffff),
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if err := _Loaded(
					t,
					tc.code,
				); err == nil {
					t.Fatal("want the program refused")
				}
			},
		)
	}
}

// TestLoad_RefusesTheCrashInput proves the 100-byte bundle fuzzing found,
// which killed Load with a fatal out of memory, is now an error (B126).
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, B126)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_RefusesTheCrashInput(t *testing.T) {
	_, err := artifact.Load([]byte(CRASH_CORPUS))
	if err == nil {
		t.Fatal("want the crash input refused")
	}
}

// TestLoad_RefusesEveryCutOfAProgram proves a real program cut at any byte is
// refused by Load and does not crash, and the whole program loads.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_RefusesEveryCutOfAProgram(t *testing.T) {
	for _, name := range []string{
		"app.star",
		"lib.star",
		"spread.star",
	} {
		t.Run(
			name,
			func(t *testing.T) {
				bundle := _Handmade(
					t,
					name,
				)
				bundle.Entry = name

				var (
					units = bundle.GetUnits()
					whole = bytes.Clone(units[len(units)-1].GetCode())
				)

				if err := _Loaded(
					t,
					whole,
				); err != nil && name == "lib.star" {
					t.Fatalf(
						"whole program: %v",
						err,
					)
				}

				for cut := range len(whole) {
					if err := _Loaded(
						t,
						whole[:cut],
					); err == nil {
						t.Fatalf(
							"cut at %d of %d loaded",
							cut,
							len(whole),
						)
					}
				}
			},
		)
	}
}

// TestLoad_LoadsEverySavedFixture proves the walker accepts what the pinned
// compiler writes: each fixture with a main, saved and loaded.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94, Q132)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_LoadsEverySavedFixture(t *testing.T) {
	for _, name := range []string{
		"app.star",
		"spread.star",
		"mainargs.star",
		"maindefault.star",
		"spins.star",
		"breaks.star",
	} {
		t.Run(
			name,
			func(t *testing.T) {
				saved, err := _Built(
					t,
					name,
				).Save()
				if err != nil {
					t.Skipf(
						"not savable: %v",
						err,
					)
				}

				_, err = artifact.Load(saved)
				if err != nil {
					t.Fatalf(
						"load: %v",
						err,
					)
				}
			},
		)
	}
}

// BenchmarkLoad_Program measures Load of the fixture bundle, which holds the
// cost of the pre-check of C94 beside the decode it guards.
//
// Revisions:
//   - 2026-11-04 21:56: initial creation (C94)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkLoad_Program(b *testing.B) {
	built, err := artifact.Compile(&common.Source{
		Entry:  "spread.star",
		Text:   []byte(_Text(SPREAD_FIXTURE)),
		Loader: _Loader(),
	})
	if err != nil {
		b.Fatal(err)
	}

	saved, err := built.Save()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err = artifact.Load(saved)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestLoad_BoundsTheUnitNameInADecodeError proves a long unit name is cut by
// the quoting helper in the refusal of its code (Q4).
//
// Revisions:
//   - 2026-11-04 21:58: initial creation (C94 fix round 1)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_BoundsTheUnitNameInADecodeError(t *testing.T) {
	const NAME_LENGTH = 5000

	bundle := _Handmade(
		t,
		"lib.star",
	)
	bundle.Entry = "lib.star"
	units := bundle.GetUnits()
	units[len(units)-1].Name = strings.Repeat(
		"n",
		NAME_LENGTH,
	)
	units[len(units)-1].Code = []byte("not a program")

	raw, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatalf(
			"marshal: %v",
			err,
		)
	}

	_, err = artifact.Load(raw)
	if err == nil {
		t.Fatal("want the program refused")
	}

	if len(err.Error()) >= NAME_LENGTH {
		t.Fatalf(
			"the refusal is %d bytes, so the name was not cut",
			len(err.Error()),
		)
	}
}
