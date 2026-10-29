// These tests hold Encode and Decode to the code they replaced, copied below
// as it was: the same text out, the same bytes back, and the same refusal, word
// for word, for the same input. A rewrite that is faster must not also be
// different, and what it refuses is part of what it does.
//
// The copy is the golden answer. It changes only if the contract does.
package binary_test

import (
	stdbinary "encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
)

const (
	// SEED fixes the random input, so a failure can be run again.
	SEED = 2026

	// WIDEST is the longest bit text tried in every spelling: 8191 texts, so
	// every short head is tried with every full group after it.
	WIDEST = 12
)

var (
	// LENGTHS are the sizes tried beyond the short ones: whole groups, and
	// one either side of them.
	LENGTHS = []int{
		23,
		24,
		25,
		63,
		64,
		65,
		127,
		128,
		129,
	}

	// NOT_BITS are characters that are not bits. Each of the first differs from
	// 0 in one bit, from the second to the seventh, so every bit the word check
	// compares is tried; the rest set the eighth bit, or are two bytes.
	NOT_BITS = []string{
		"2",
		"4",
		"8",
		" ",
		"\x10",
		"p",
		"/",
		"\x00",
		"\xb0",
		"\xff",
		"é",
	}
)

// TestEncode_MatchesPreviousCode gives Encode every count of bytes up to 17,
// and some longer, and expects the text the previous code wrote.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func TestEncode_MatchesPreviousCode(t *testing.T) {
	random := rand.New(
		rand.NewPCG(
			SEED,
			SEED,
		),
	)

	for _, size := range _Sizes() {
		t.Run(
			fmt.Sprint(size),
			func(t *testing.T) {
				data := make(
					[]byte,
					size,
				)
				for idx := range data {
					data[idx] = byte(random.Uint32())
				}

				var (
					got  = binary.Encode(data)
					want = _PreviousEncode(data)
				)

				if got != want {
					t.Fatalf(
						"Encode(%x) gave %q, want %q",
						data,
						got,
						want,
					)
				}
			},
		)
	}
}

// TestDecode_MatchesPreviousCode gives Decode every bit text up to WIDEST
// characters, and random ones of every count up to 17 and longer, and expects
// the bytes the previous code read: the same values, and empty but not nil
// for empty text.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func TestDecode_MatchesPreviousCode(t *testing.T) {
	random := rand.New(
		rand.NewPCG(
			SEED,
			SEED,
		),
	)

	var texts []string

	for width := range WIDEST + 1 {
		for value := range 1 << width {
			texts = append(
				texts,
				_Spelled(
					value,
					width,
				),
			)
		}
	}

	for _, size := range _Sizes() {
		var text strings.Builder

		for range size {
			text.WriteByte('0' + byte(random.IntN(2)))
		}

		texts = append(
			texts,
			text.String(),
		)
	}

	for _, text := range texts {
		got, err := binary.Decode(
			WHO,
			text,
		)
		if err != nil {
			t.Fatalf(
				"Decode(%q): %v",
				text,
				err,
			)
		}

		want, err := _PreviousDecode(
			WHO,
			text,
		)
		if err != nil {
			t.Fatalf(
				"previous Decode(%q): %v",
				text,
				err,
			)
		}

		same := string(got) == string(want) && (got == nil) == (want == nil)
		if !same {
			t.Fatalf(
				"Decode(%q) gave %#v, want %#v",
				text,
				got,
				want,
			)
		}
	}
}

// TestDecode_RefusesAsPreviousCodeDid puts each character of NOT_BITS at every
// position of texts of every length up to 17, and longer, so it falls in the
// short head and in every place of a full group, and expects the refusal the
// previous code gave: ErrBits, with the same message.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func TestDecode_RefusesAsPreviousCodeDid(t *testing.T) {
	for _, size := range _Sizes() {
		t.Run(
			fmt.Sprint(size),
			func(t *testing.T) {
				for _, wrong := range NOT_BITS {
					for at := range size {
						_Refuses(
							t,
							strings.Repeat(
								"1",
								at,
							)+wrong+strings.Repeat(
								"0",
								size-1-at,
							),
						)
					}
				}
			},
		)
	}
}

// _Refuses decodes text that is not bits with Decode and with the previous
// code, and expects both to refuse it with ErrBits and the same message.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func _Refuses(
	t *testing.T,
	text string,
) {
	t.Helper()

	got, err := binary.Decode(
		WHO,
		text,
	)
	if !errors.Is(
		err,
		binary.ErrBits,
	) {
		t.Fatalf(
			"Decode(%q) gave %v and %v, want %v",
			text,
			got,
			err,
			binary.ErrBits,
		)
	}

	_, want := _PreviousDecode(
		WHO,
		text,
	)
	if err.Error() != want.Error() {
		t.Fatalf(
			"Decode(%q) refused with %q, want %q",
			text,
			err,
			want,
		)
	}

	if got != nil {
		t.Fatalf(
			"Decode(%q) gave %#v with its refusal, want nil",
			text,
			got,
		)
	}
}

// _Sizes is every count from 0 to 17, which is two full groups and every
// short head, then LENGTHS.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func _Sizes() []int {
	var sizes []int

	for size := range 2*binary.BITS_PER_BYTE + 2 {
		sizes = append(
			sizes,
			size,
		)
	}

	return append(
		sizes,
		LENGTHS...,
	)
}

// _Spelled is value as width bits, the most significant first, padded with
// zeros on the left.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation
func _Spelled(
	value int,
	width int,
) string {
	text := make(
		[]byte,
		width,
	)

	for idx := range text {
		text[idx] = '0' + byte(value>>(width-1-idx)&1)
	}

	return string(text)
}

// _PreviousEncode is Encode as it was before it stored each word in place.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from Encode
func _PreviousEncode(data []byte) string {
	var (
		out    strings.Builder
		digits [binary.BITS_PER_BYTE]byte
	)

	out.Grow(len(data) * binary.BITS_PER_BYTE)

	for _, octet := range data {
		stdbinary.LittleEndian.PutUint64(
			digits[:],
			_PreviousDigits(octet),
		)
		out.Write(digits[:])
	}

	return out.String()
}

// _PreviousDecode is Decode as it was before it read the text in one pass.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from Decode
func _PreviousDecode(
	who string,
	bits string,
) ([]byte, error) {
	err := _PreviousCheck(
		who,
		bits,
	)
	if err != nil {
		return nil, err
	}

	var (
		padded = _PreviousPadded(bits)
		data   = make(
			[]byte,
			0,
			len(padded)/binary.BITS_PER_BYTE,
		)
	)

	for at := 0; at < len(padded); at += binary.BITS_PER_BYTE {
		data = append(
			data,
			_PreviousByteOfBits(padded[at:at+binary.BITS_PER_BYTE]),
		)
	}

	return data, nil
}

// _PreviousCheck is _Check as it was.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from _Check
func _PreviousCheck(
	who string,
	text string,
) error {
	whole := len(text) - len(text)%binary.BITS_PER_BYTE

	for at := 0; at < whole; at += binary.BITS_PER_BYTE {
		digits := _PreviousWord(text[at:at+binary.BITS_PER_BYTE])&binary.SHARED ==
			binary.ZEROS
		if !digits {
			return fmt.Errorf(
				"%s got %q: %w",
				who,
				text,
				binary.ErrBits,
			)
		}
	}

	for _, char := range text[whole:] {
		if char != '0' && char != '1' {
			return fmt.Errorf(
				"%s got %q: %w",
				who,
				text,
				binary.ErrBits,
			)
		}
	}

	return nil
}

// _PreviousPadded is _Padded as it was.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from _Padded
func _PreviousPadded(bits string) string {
	short := len(bits) % binary.BITS_PER_BYTE
	if short == 0 {
		return bits
	}

	return strings.Repeat(
		binary.PAD,
		binary.BITS_PER_BYTE-short,
	) + bits
}

// _PreviousByteOfBits is _ByteOfBits as it was, reading eight characters.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from _ByteOfBits
func _PreviousByteOfBits(bits string) byte {
	gathered := (_PreviousWord(bits) - binary.ZEROS) * binary.GATHER

	return byte(gathered >> binary.TOP_BYTE)
}

// _PreviousDigits is _Digits as it was.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from _Digits
func _PreviousDigits(octet byte) uint64 {
	var (
		kept   = (uint64(octet) * binary.LANES) & binary.SPREAD
		lifted = (kept + binary.CARRY) >> (binary.BITS_PER_BYTE - 1)
	)

	return (lifted & binary.LANES) | binary.ZEROS
}

// _PreviousWord is _Word as it was.
//
// Revisions:
//   - 2026-10-28 23:03: initial creation, copied from _Word
func _PreviousWord(text string) uint64 {
	return stdbinary.LittleEndian.Uint64([]byte(text))
}
