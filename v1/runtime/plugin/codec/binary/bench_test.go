package binary_test

import (
	"strconv"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
)

const (
	// WHO is the name a refusal would give. The benchmark text is all bits,
	// so no refusal comes.
	WHO = "bench"
)

var (
	// SIZES are the counts of bytes that each benchmark encodes or decodes.
	SIZES = []int{
		16,
		1024,
		65536,
	}
)

// BenchmarkEncode measures Encode on data of each size. The bytes per second
// are bytes of data, the input.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkEncode(b *testing.B) {
	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				data := _Data(size)

				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					binary.Encode(data)
				}
			},
		)
	}
}

// BenchmarkDecode measures Decode on the encoding of data of each size. The
// bytes per second are characters of text, the input, eight to a byte of
// data.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func BenchmarkDecode(b *testing.B) {
	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				text := binary.Encode(_Data(size))

				b.SetBytes(int64(len(text)))
				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					_, err := binary.Decode(
						WHO,
						text,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// _Data is size bytes that run through every byte value, so each value of a
// byte is in the input once size is at least 256.
//
// Revisions:
//   - 2026-10-28 21:56: initial creation
func _Data(size int) []byte {
	data := make(
		[]byte,
		size,
	)

	for idx := range data {
		data[idx] = byte(idx)
	}

	return data
}
