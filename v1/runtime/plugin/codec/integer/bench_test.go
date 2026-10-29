package integer_test

import (
	"math/big"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// WHO is the name a refusal would give. Every benchmark number fits its
	// width, so no refusal comes.
	WHO = "bench"
)

// _Shape is one field a benchmark writes and reads: its width in bytes, its
// byte order, and whether it is two's complement.
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
	SHAPES = []*_Shape{
		{
			name:  "unsigned/big/1",
			width: 1,
			order: integer.BIG,
		},
		{
			name:  "unsigned/big/4",
			width: 4,
			order: integer.BIG,
		},
		{
			name:  "unsigned/big/8",
			width: 8,
			order: integer.BIG,
		},
		{
			name:  "unsigned/big/16",
			width: 16,
			order: integer.BIG,
		},
		{
			name:  "unsigned/little/1",
			width: 1,
			order: integer.LITTLE,
		},
		{
			name:  "unsigned/little/4",
			width: 4,
			order: integer.LITTLE,
		},
		{
			name:  "unsigned/little/8",
			width: 8,
			order: integer.LITTLE,
		},
		{
			name:  "unsigned/little/16",
			width: 16,
			order: integer.LITTLE,
		},
		{
			name:   "signed/big/8",
			width:  8,
			order:  integer.BIG,
			signed: true,
		},
		{
			name:   "signed/little/8",
			width:  8,
			order:  integer.LITTLE,
			signed: true,
		},
	}
)

// BenchmarkEncode measures the write of one field of each shape, through
// Encode or EncodeSigned, as integer.to_bytes writes it.
//
// Revisions:
//   - 2026-10-29 00:35: initial creation
func BenchmarkEncode(b *testing.B) {
	budget := scheduler.NewBudget(scheduler.CEILING)

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
				b.ResetTimer()

				for range b.N {
					_, err := encode(
						budget,
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
func BenchmarkDecode(b *testing.B) {
	budget := scheduler.NewBudget(scheduler.CEILING)

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
					budget,
					WHO,
					_Field(shape),
				)
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
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
func _Field(shape *_Shape) *integer.Field {
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

	return &integer.Field{
		Number: number,
		Width:  shape.width,
		Order:  shape.order,
	}
}
