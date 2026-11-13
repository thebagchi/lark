package tests

import (
	"bytes"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
)

// FuzzDecode feeds Decode and DecodeSigned any bytes in any order name. An
// order other than big and little is ErrOrder and never a panic. A good order
// reads to an integer that Encode and EncodeSigned write back to the bytes,
// at the width of the input: unsigned for Decode, two's complement for
// DecodeSigned. Little reads the reverse of big. The seeds are the table
// cases of integer_test.go, the word edges, and the first width past a word.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func FuzzDecode(f *testing.F) {
	for _, seed := range []struct {
		data  []byte
		order string
	}{
		{
			nil,
			common.BIG,
		},
		{
			[]byte{0},
			common.BIG,
		},
		{
			[]byte{1, 0},
			common.BIG,
		},
		{
			[]byte{2, 1},
			common.LITTLE,
		},
		{
			[]byte{0xff},
			common.BIG,
		},
		{
			[]byte{0x80},
			common.LITTLE,
		},
		{
			bytes.Repeat(
				[]byte{0xff},
				8,
			),
			common.BIG,
		},
		{
			append(
				[]byte{0x80},
				make(
					[]byte,
					7,
				)...,
			),
			common.BIG,
		},
		{
			append(
				[]byte{1},
				make(
					[]byte,
					8,
				)...,
			),
			common.BIG,
		},
		{
			bytes.Repeat(
				[]byte{0xff},
				9,
			),
			common.LITTLE,
		},
		{
			append(
				[]byte{0x80},
				make(
					[]byte,
					16,
				)...,
			),
			common.BIG,
		},
		{
			[]byte{1},
			"middle",
		},
		{
			nil,
			"",
		},
		{
			[]byte{1, 2, 3},
			"BIG",
		},
	} {
		f.Add(
			seed.data,
			seed.order,
		)
	}

	f.Fuzz(func(
		t *testing.T,
		data []byte,
		order string,
	) {
		valid := order == common.BIG || order == common.LITTLE

		var (
			unsigned, uerr = integer.Decode(
				"fuzz",
				data,
				order,
			)
			signed, serr = integer.DecodeSigned(
				"fuzz",
				data,
				order,
			)
		)

		if !valid {
			if uerr == nil || serr == nil {
				t.Fatalf(
					"order %q accepted: %v, %v",
					order,
					uerr,
					serr,
				)
			}

			return
		}

		if uerr != nil || serr != nil {
			t.Fatalf(
				"decode(%x, %s) refused: %v, %v",
				data,
				order,
				uerr,
				serr,
			)
		}

		if len(data) == 0 {
			return
		}

		for _, item := range []struct {
			name   string
			number starlark.Int
			encode func(
				string,
				*common.Field,
			) ([]byte, error)
		}{
			{
				"Decode",
				unsigned,
				integer.Encode,
			},
			{
				"DecodeSigned",
				signed,
				integer.EncodeSigned,
			},
		} {
			back, err := item.encode(
				"fuzz",
				&common.Field{
					Number: item.number,
					Width:  len(data),
					Order:  order,
				},
			)
			if err != nil {
				t.Fatalf(
					"%s(%x, %s) = %s, encode refused: %v",
					item.name,
					data,
					order,
					item.number,
					err,
				)
			}

			if !bytes.Equal(
				back,
				data,
			) {
				t.Fatalf(
					"%s(%x, %s) = %s, encodes to %x",
					item.name,
					data,
					order,
					item.number,
					back,
				)
			}
		}
	})
}
