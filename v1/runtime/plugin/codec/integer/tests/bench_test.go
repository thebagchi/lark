package tests

import (
	"math/big"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
)

const (
	// WHO is the name a refusal would give. Every benchmark number fits its
	// width, so no refusal comes.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WHO = "bench"

	// BENCH is the name of the thread that each builtin benchmark calls on.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BENCH = "bench"
)

var (
	// WIDTHS are the byte counts of the builtin benchmarks: 2 and 8 are the
	// widths the plan's measurements were taken at, 16 is past a machine word.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WIDTHS = []int{
		2,
		8,
		16,
	}
)

// _Shape is one field a benchmark writes and reads: its width in bytes, its
// byte order, and whether it is two's complement.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Shape struct {
	name   string
	width  int
	order  string
	signed bool
}

var (
	// SHAPES are the fields that each benchmark writes or reads. Most codec
	// fields are one to eight bytes; sixteen is one past what a machine word
	// holds.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SHAPES = []*_Shape{
		{
			name:  "unsigned/big/1",
			width: 1,
			order: common.BIG,
		},
		{
			name:  "unsigned/big/4",
			width: 4,
			order: common.BIG,
		},
		{
			name:  "unsigned/big/8",
			width: 8,
			order: common.BIG,
		},
		{
			name:  "unsigned/big/16",
			width: 16,
			order: common.BIG,
		},
		{
			name:  "unsigned/little/1",
			width: 1,
			order: common.LITTLE,
		},
		{
			name:  "unsigned/little/4",
			width: 4,
			order: common.LITTLE,
		},
		{
			name:  "unsigned/little/8",
			width: 8,
			order: common.LITTLE,
		},
		{
			name:  "unsigned/little/16",
			width: 16,
			order: common.LITTLE,
		},
		{
			name:   "signed/big/8",
			width:  8,
			order:  common.BIG,
			signed: true,
		},
		{
			name:   "signed/little/8",
			width:  8,
			order:  common.LITTLE,
			signed: true,
		},
	}
)

// BenchmarkEncode measures the write of one field of each shape, through
// Encode or EncodeSigned, as integer.to_bytes writes it.
//
// Revisions:
//   - 2026-10-29 00:35: initial creation
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEncode(b *testing.B) {
	for _, shape := range SHAPES {
		b.Run(
			shape.name,
			func(b *testing.B) {
				encode := integer.Encode
				if shape.signed {
					encode = integer.EncodeSigned
				}

				field := _Field(shape)

				b.ReportAllocs()

				for b.Loop() {
					_, err := encode(
						WHO,
						field,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkDecode measures the read of one field of each shape, through Decode
// or DecodeSigned, as integer.from_bytes reads it.
//
// Revisions:
//   - 2026-10-29 00:35: initial creation
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-01 22:10: measure with b.Loop in place of a b.N loop; drop b.ResetTimer
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkDecode(b *testing.B) {
	for _, shape := range SHAPES {
		b.Run(
			shape.name,
			func(b *testing.B) {
				var (
					encode = integer.Encode
					decode = integer.Decode
				)
				if shape.signed {
					encode = integer.EncodeSigned
					decode = integer.DecodeSigned
				}

				data, err := encode(
					WHO,
					_Field(shape),
				)
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()

				for b.Loop() {
					_, err = decode(
						WHO,
						data,
						shape.order,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _Field is a field of shape whose number has every byte set: the bytes 1, 2,
// and on, big-endian. A signed shape gets that number negated, so a benchmark
// also pays for the complement that a negative number needs.
//
// Revisions:
//   - 2026-10-29 00:35: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Field(shape *_Shape) *common.Field {
	data := make(
		[]byte,
		shape.width,
	)

	for idx := range data {
		data[idx] = byte(idx + 1)
	}

	number := new(big.Int).SetBytes(data)
	if shape.signed {
		number.Neg(number)
	}

	return &common.Field{
		Number: starlark.MakeBigInt(number),
		Width:  shape.width,
		Order:  shape.order,
	}
}

// BenchmarkFromBytes_Builtin measures integer.from_bytes on bytes of each
// width, through the builtin, so the reading of its arguments shows in the
// allocations.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkFromBytes_Builtin(b *testing.B) {
	fn := _Builtin(
		b,
		"from_bytes",
	)

	for _, width := range WIDTHS {
		b.Run(
			strconv.Itoa(width),
			func(b *testing.B) {
				data := make(
					[]byte,
					width,
				)

				for idx := range data {
					data[idx] = byte(idx + 1)
				}

				_Call(
					b,
					fn,
					starlark.Tuple{
						starlark.Bytes(data),
					},
					nil,
				)
			},
		)
	}
}

// BenchmarkToBytes_Builtin measures integer.to_bytes on the number 300 padded
// to each width, through the builtin.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkToBytes_Builtin(b *testing.B) {
	fn := _Builtin(
		b,
		"to_bytes",
	)

	for _, width := range WIDTHS {
		b.Run(
			strconv.Itoa(width),
			func(b *testing.B) {
				_Call(
					b,
					fn,
					starlark.Tuple{
						starlark.MakeInt(300),
						starlark.MakeInt(width),
					},
					nil,
				)
			},
		)
	}
}

// _Builtin is the member name of the integer module, found in the environment a
// script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-03 06:45: takes a testing.TB, so a test finds the builtin too (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Builtin(
	tb testing.TB,
	name string,
) *starlark.Builtin {
	tb.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		tb.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env["integer"].(*starlarkstruct.Module)
	if !ok {
		tb.Fatalf(
			"%s is not a module",
			"integer",
		)
	}

	fn, ok := module.Members[name].(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"%s.%s is not a builtin",
			"integer",
			name,
		)
	}

	return fn
}

// _Call calls fn on args once per iteration, as a script's call reaches it,
// and ends the benchmark at the first error.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Call(
	b *testing.B,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) {
	b.Helper()

	thread := &starlark.Thread{
		Name: BENCH,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := fn.CallInternal(
			thread,
			args,
			kwargs,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
