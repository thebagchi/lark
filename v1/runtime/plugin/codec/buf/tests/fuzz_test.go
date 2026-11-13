package tests

import (
	"bytes"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
)

// FuzzReader feeds buf.reader any bytes and any run of reads, one op byte
// each. A read that succeeds takes exactly its width from the front, and
// gives the integer that integer.Decode or integer.DecodeSigned gives for
// those bytes, or the bytes themselves (bytes for read_bytes, a str for
// read_string; the op byte gives the count). A read that fails takes nothing.
// No run panics. The seeds are the reads of buf_test.go.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-04 19:33: fuzzes read_bytes and read_string too (C89 fix round 1)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func FuzzReader(f *testing.F) {
	for _, seed := range []struct {
		data []byte
		ops  []byte
	}{
		{
			nil,
			[]byte{0},
		},
		{
			[]byte("ab"),
			[]byte{0, 0, 0},
		},
		{
			[]byte{1, 2, 3, 4, 5, 6, 7, 8},
			[]byte{3},
		},
		{
			[]byte{0xff, 0xfe, 0xfd, 0xfc},
			[]byte{6, 7},
		},
		{
			bytes.Repeat(
				[]byte{0x80},
				16,
			),
			[]byte{3, 11, 15, 8, 9},
		},
		{
			[]byte("hello"),
			[]byte{8, 9, 10},
		},
		{
			[]byte("hello world"),
			[]byte{88, 89, 98, 3, 88},
		},
	} {
		f.Add(
			seed.data,
			seed.ops,
		)
	}

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		f.Fatalf(
			"environment: %v",
			err,
		)
	}

	// ops names the method of each op value, its width and its sign.
	var (
		module       = env["buf"].(starlark.HasAttrs)
		newReader, _ = module.Attr("reader")
		ops          = []struct {
			name   string
			width  int
			signed bool
		}{
			{
				"read_uint8",
				1,
				false,
			},
			{
				"read_uint16",
				2,
				false,
			},
			{
				"read_uint32",
				4,
				false,
			},
			{
				"read_uint64",
				8,
				false,
			},
			{
				"read_int8",
				1,
				true,
			},
			{
				"read_int16",
				2,
				true,
			},
			{
				"read_int32",
				4,
				true,
			},
			{
				"read_int64",
				8,
				true,
			},
		}
	)

	f.Fuzz(func(
		t *testing.T,
		data []byte,
		run []byte,
	) {
		value, err := starlark.Call(
			&starlark.Thread{},
			newReader,
			starlark.Tuple{starlark.Bytes(data)},
			nil,
		)
		if err != nil {
			t.Fatalf(
				"reader(%x): %v",
				data,
				err,
			)
		}

		var (
			reader = value.(*buffer.Reader)
			at     = 0
		)

		for _, op := range run {
			if kind := int(op) % (len(ops) + 2); kind >= len(ops) {
				name := "read_bytes"
				if kind > len(ops) {
					name = "read_string"
				}

				at += _Taken(
					t,
					reader,
					data[at:],
					&_Take{
						name:  name,
						count: int(op>>4) % 17,
					},
				)

				continue
			}

			var (
				spec      = ops[int(op)%len(ops)]
				method, _ = reader.Attr(spec.name)
				before    = reader.Len()
			)

			got, err := starlark.Call(
				&starlark.Thread{},
				method,
				nil,
				nil,
			)
			if err != nil {
				if reader.Len() != before {
					t.Fatalf(
						"%s failed and moved the reader: %d to %d",
						spec.name,
						before,
						reader.Len(),
					)
				}

				if before >= spec.width {
					t.Fatalf(
						"%s refused with %d bytes left: %v",
						spec.name,
						before,
						err,
					)
				}

				continue
			}

			decode := integer.Decode
			if spec.signed {
				decode = integer.DecodeSigned
			}

			want, err := decode(
				"fuzz",
				data[at:at+spec.width],
				common.BIG,
			)
			if err != nil {
				t.Fatalf(
					"decode %x: %v",
					data[at:at+spec.width],
					err,
				)
			}

			if equal, _ := starlark.Equal(
				got,
				want,
			); !equal {
				t.Fatalf(
					"%s at %d gave %s, want %s",
					spec.name,
					at,
					got,
					want,
				)
			}

			at += spec.width

			if reader.Len() != len(data)-at {
				t.Fatalf(
					"%s left %d bytes, want %d",
					spec.name,
					reader.Len(),
					len(data)-at,
				)
			}
		}
	})
}

// _Take names a read of count bytes: read_bytes or read_string.
//
// Revisions:
//   - 2026-11-04 21:06: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Take struct {
	name  string
	count int
}

// _Taken calls the read take names, for take.count bytes of
// rest, the bytes not yet read, and ends the test unless it gives exactly the
// next count bytes, or fails with the reader unmoved because too few remain.
// It returns how many bytes the read took.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89 fix round 1)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Taken(
	t *testing.T,
	reader *buffer.Reader,
	rest []byte,
	take *_Take,
) int {
	t.Helper()

	var (
		name      = take.name
		count     = take.count
		text      = name == "read_string"
		method, _ = reader.Attr(name)
		before    = reader.Len()
	)

	got, err := starlark.Call(
		&starlark.Thread{},
		method,
		starlark.Tuple{starlark.MakeInt(count)},
		nil,
	)
	if err != nil {
		if reader.Len() != before || before >= count {
			t.Fatalf(
				"%s(%d) with %d left gave %v, reader now %d",
				name,
				count,
				before,
				err,
				reader.Len(),
			)
		}

		return 0
	}

	var want starlark.Value = starlark.Bytes(rest[:count])
	if text {
		want = starlark.String(rest[:count])
	}

	if equal, _ := starlark.Equal(
		got,
		want,
	); !equal || reader.Len() != before-count {
		t.Fatalf(
			"%s(%d) gave %v, want %v, left %d of %d",
			name,
			count,
			got,
			want,
			reader.Len(),
			before,
		)
	}

	return count
}
