package tests

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
)

// FuzzDecode feeds Decode any text. Text of 0 and 1 only decodes to
// ceil(len/8) bytes, and Encode of them is the text left-padded with zeros to
// whole bytes. Any other text is ErrBits and never a panic. The seeds are the
// table cases of binary_test.go: empty, short heads, whole groups, and bad
// digits in the head and in a group.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"", "0", "1", "01", "101", "11111111", "00000000", "100000001",
		"1111111111", "0000000000000000", "010101010101010101010101",
		"2", "a", " ", "0 1", "1111111x", "x1111111", "11111112",
		"1111111111111112", "\x00", "\xff", "0b101", "１",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		data, err := binary.Decode(
			"fuzz",
			text,
		)
		if strings.Trim(
			text,
			"01",
		) != "" {
			if !errors.Is(
				err,
				binary.ErrBits,
			) {
				t.Fatalf(
					"Decode(%q) gave %v, want ErrBits",
					text,
					err,
				)
			}

			return
		}

		if err != nil {
			t.Fatalf(
				"Decode(%q) refused: %v",
				text,
				err,
			)
		}

		if want := (len(text) + 7) / 8; len(data) != want {
			t.Fatalf(
				"Decode(%q) gave %d bytes, want %d",
				text,
				len(data),
				want,
			)
		}

		padded := strings.Repeat(
			"0",
			len(data)*8-len(text),
		) + text
		if got := binary.Encode(data); got != padded {
			t.Fatalf(
				"Encode(Decode(%q)) = %q, want %q",
				text,
				got,
				padded,
			)
		}

		if !bytes.Equal(
			data,
			_Decoded(
				t,
				padded,
			),
		) {
			t.Fatalf(
				"Decode(%q) differs from Decode of its padding",
				text,
			)
		}
	})
}

// _Decoded decodes whole bytes of bits or ends the test.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Decoded(
	t *testing.T,
	bits string,
) []byte {
	t.Helper()

	data, err := binary.Decode(
		"fuzz",
		bits,
	)
	if err != nil {
		t.Fatalf(
			"Decode(%q): %v",
			bits,
			err,
		)
	}

	return data
}
