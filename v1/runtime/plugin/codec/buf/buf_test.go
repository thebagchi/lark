// These tests are table driven because the thing under test is a
// specification: what each method writes or reads, one row each, so a missing
// rule is visible as a missing row. A row is a few statements on one line, and
// its answer is what they leave in out.
package buf_test

import (
	"errors"
	"maps"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// SCRIPT is what a failure calls the script it was given, and OUT the
	// global each row leaves its answer in.
	SCRIPT = "buf_test.star"
	OUT    = "out"

	// MADE is a script making a buffer and a reader at its top level, which
	// Starlark freezes once the script ends.
	MADE = `
b = buf.new()
b.write_bytes("ab")
r = buf.reader("ab")
`
)

// _Environment is the plugin environment a script here runs against.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Environment(t *testing.T) starlark.StringDict {
	t.Helper()

	env, err := plugin.Environment(plugin.DEFAULT)
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	return env
}

// _Run runs src against the plugin environment and returns the String form of
// what it left in out.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Run(t *testing.T, src string) (string, error) {
	t.Helper()

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		src,
		_Environment(t),
	)
	if err != nil {
		return "", err
	}

	out, ok := globals[OUT]
	if !ok {
		t.Fatalf("%s left nothing in %s", src, OUT)
	}

	return out.String(), nil
}

// _Check runs a table of script / expected pairs.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Check(t *testing.T, cases []struct{ name, src, want string }) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Run(t, item.src)
			if err != nil {
				t.Fatalf("%s: %v", item.src, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.src, got, item.want)
			}
		})
	}
}

// _Refuse runs a table of scripts that must fail with the sentinel given.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Refuse(t *testing.T, cases []struct {
	name string
	src  string
	want error
}) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Run(t, item.src)
			if !errors.Is(err, item.want) {
				t.Fatalf("%s gave %v, want %v", item.src, err, item.want)
			}
		})
	}
}

// TestBuffer_WritesBytesAndStrings is bytes, a str's bytes through either
// method, in the order they are written, and a copy each time they are read
// back.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as TestBuffer_WritesWhatItIsGiven
//   - 2026-10-08 21:37: write_bytes and write_string, where write was
func TestBuffer_WritesBytesAndStrings(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{
			"bytes, and a str either way",
			`b = buf.new(); b.write_bytes(b"\x01"); b.write_bytes("A"); ` +
				`b.write_string("B"); out = b.bytes()`,
			`b"\x01AB"`,
		},
		{"nothing", `b = buf.new(); out = b.bytes()`, `b""`},
		{
			"a copy, which later writes leave alone",
			`b = buf.new(); b.write_bytes("A"); first = b.bytes(); ` +
				`b.write_bytes("B"); out = [first, b.bytes()]`,
			`[b"A", b"AB"]`,
		},
	})

	_Refuse(t, []struct {
		name string
		src  string
		want error
	}{
		{"a number", `b = buf.new(); b.write_bytes(1)`, unpack.ERR_DATA},
	})
}

// TestBuffer_WritesIntegersOfAnyWidth is integer's rules in the width a script
// names: unsigned for write_uint, two's complement for write_int, big-endian
// unless "little" is named, and refused when a number does not fit, an order is
// unknown, or a width is past the run.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as TestBuffer_WritesIntegers
//   - 2026-10-08 21:37: write_uint and write_int, where uint and int were, and a
//     width of three
func TestBuffer_WritesIntegersOfAnyWidth(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{
			"uint",
			`b = buf.new(); b.write_uint(258, 2); out = b.bytes()`,
			`b"\x01\x02"`,
		},
		{
			"uint, big named",
			`b = buf.new(); b.write_uint(258, 2, "big"); out = b.bytes()`,
			`b"\x01\x02"`,
		},
		{
			"uint, little",
			`b = buf.new(); b.write_uint(258, 2, "little"); out = b.bytes()`,
			`b"\x02\x01"`,
		},
		{
			"uint in three bytes",
			`b = buf.new(); b.write_uint(258, 3); out = b.bytes()`,
			`b"\x00\x01\x02"`,
		},
		{
			"int of -1 in three bytes, little",
			`b = buf.new(); b.write_int(-1, 3, "little"); out = b.bytes()`,
			`b"\xff\xff\xff"`,
		},
		{
			"one after another",
			`b = buf.new(); b.write_bytes("A"); b.write_uint(1, 2); ` +
				`b.write_int(-1, 1); out = b.bytes()`,
			`b"A\x00\x01\xff"`,
		},
	})

	_Refuse(t, []struct {
		name string
		src  string
		want error
	}{
		{"uint negative", `b = buf.new(); b.write_uint(-1, 2)`, unpack.ERR_RANGE},
		{"uint too wide", `b = buf.new(); b.write_uint(256, 1)`, unpack.ERR_RANGE},
		{"uint of no width", `b = buf.new(); b.write_uint(0, 0)`, unpack.ERR_RANGE},
		{"int past a byte", `b = buf.new(); b.write_int(128, 1)`, unpack.ERR_RANGE},
		{
			"an order nobody has",
			`b = buf.new(); b.write_uint(1, 2, "middle")`,
			integer.ERR_ORDER,
		},
		{
			"a width past the run",
			`b = buf.new(); b.write_uint(0, 1 << 40)`,
			scheduler.ERR_MEMORY,
		},
	})
}

// TestBuffer_WritesIntegersOfFixedWidth is each of the eight fixed widths, at
// one of its ends or in an order, and refused one past an end.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func TestBuffer_WritesIntegersOfFixedWidth(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{
			"uint8 at its top",
			`b = buf.new(); b.write_uint8(255); out = b.bytes()`,
			`b"\xff"`,
		},
		{
			"uint16",
			`b = buf.new(); b.write_uint16(258); out = b.bytes()`,
			`b"\x01\x02"`,
		},
		{
			"uint16, little",
			`b = buf.new(); b.write_uint16(258, "little"); out = b.bytes()`,
			`b"\x02\x01"`,
		},
		{
			"uint32",
			`b = buf.new(); b.write_uint32(16909060); out = b.bytes()`,
			`b"\x01\x02\x03\x04"`,
		},
		{
			"uint64 at its top",
			`b = buf.new(); b.write_uint64(18446744073709551615); out = b.bytes()`,
			`b"\xff\xff\xff\xff\xff\xff\xff\xff"`,
		},
		{
			"int8 at its bottom",
			`b = buf.new(); b.write_int8(-128); out = b.bytes()`,
			`b"\x80"`,
		},
		{
			"int16, little",
			`b = buf.new(); b.write_int16(-2, "little"); out = b.bytes()`,
			`b"\xfe\xff"`,
		},
		{
			"int32 of -1",
			`b = buf.new(); b.write_int32(-1); out = b.bytes()`,
			`b"\xff\xff\xff\xff"`,
		},
		{
			"int64 at its bottom",
			`b = buf.new(); b.write_int64(-9223372036854775808); out = b.bytes()`,
			`b"\x80\x00\x00\x00\x00\x00\x00\x00"`,
		},
	})

	_Refuse(t, []struct {
		name string
		src  string
		want error
	}{
		{"uint8 past its top", `b = buf.new(); b.write_uint8(256)`, unpack.ERR_RANGE},
		{
			"uint64 past its top",
			`b = buf.new(); b.write_uint64(18446744073709551616)`,
			unpack.ERR_RANGE,
		},
		{"uint16 negative", `b = buf.new(); b.write_uint16(-1)`, unpack.ERR_RANGE},
		{"int8 past its top", `b = buf.new(); b.write_int8(128)`, unpack.ERR_RANGE},
		{
			"int64 past its bottom",
			`b = buf.new(); b.write_int64(-9223372036854775809)`,
			unpack.ERR_RANGE,
		},
		{
			"an order nobody has",
			`b = buf.new(); b.write_uint16(1, "middle")`,
			integer.ERR_ORDER,
		},
	})
}

// TestBuffer_ShowsWhatItHolds is what len, str, type, truth, an index and dir
// see of a buffer.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: the methods dir lists, named for Go's
func TestBuffer_ShowsWhatItHolds(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{"len", `b = buf.new(); b.write_bytes("abc"); out = len(b)`, `3`},
		{
			"str",
			`b = buf.new(); b.write_bytes("abc"); out = str(b)`,
			`"<buffer length 3>"`,
		},
		{"type", `out = type(buf.new())`, `"buffer"`},
		{"false when empty", `out = bool(buf.new())`, `False`},
		{"true once written", `b = buf.new(); b.write_bytes("a"); out = bool(b)`, `True`},
		{
			"a byte by index",
			`b = buf.new(); b.write_bytes("abc"); out = [b[0], b[-1]]`,
			`[b"a", b"c"]`,
		},
		{
			"its methods",
			`out = dir(buf.new())`,
			`["bytes", "write_bytes", "write_int", "write_int16", "write_int32", ` +
				`"write_int64", "write_int8", "write_string", "write_uint", ` +
				`"write_uint16", "write_uint32", "write_uint64", "write_uint8"]`,
		},
	})
}

// TestReader_TakesBytesApartInOrder is each read moving past what it read,
// integers by integer's rules in a width the script names, and a look by index
// moving nothing.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: read_bytes, read_uint and read_int, where read, uint and
//     int were, and read_string
func TestReader_TakesBytesApartInOrder(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{
			"integers and bytes in turn",
			`r = buf.reader(b"\x01\x02\x03\x04AB"); out = [r.read_uint(2), ` +
				`r.read_uint(2, "little"), r.read_bytes(2), len(r)]`,
			`[258, 1027, b"AB", 0]`,
		},
		{
			"signed",
			`r = buf.reader(b"\xff\xfe"); out = [r.read_int(1), r.read_int(1)]`,
			`[-1, -2]`,
		},
		{
			"signed in three bytes, little",
			`r = buf.reader(b"\xfe\xff\xff"); out = r.read_int(3, "little")`,
			`-2`,
		},
		{"bytes from a str", `r = buf.reader("AB"); out = r.read_bytes(2)`, `b"AB"`},
		{"a str", `r = buf.reader(b"AB"); out = r.read_string(2)`, `"AB"`},
		{
			"nothing",
			`r = buf.reader(b"abc"); out = [r.read_bytes(0), len(r)]`,
			`[b"", 3]`,
		},
		{
			"a look does not move it",
			`r = buf.reader("AB"); out = [r[0], r[1], len(r)]`,
			`[b"A", b"B", 2]`,
		},
		{
			"str",
			`r = buf.reader("AB"); r.read_bytes(1); out = str(r)`,
			`"<reader length 1>"`,
		},
		{"type", `out = type(buf.reader(""))`, `"reader"`},
		{
			"false when read out",
			`r = buf.reader("A"); r.read_bytes(1); out = bool(r)`,
			`False`,
		},
		{
			"its methods",
			`out = dir(buf.reader(""))`,
			`["read_bytes", "read_int", "read_int16", "read_int32", "read_int64", ` +
				`"read_int8", "read_string", "read_uint", "read_uint16", ` +
				`"read_uint32", "read_uint64", "read_uint8"]`,
		},
	})

	_Refuse(t, []struct {
		name string
		src  string
		want error
	}{
		{
			"an integer past the end",
			`r = buf.reader(b"\x01"); r.read_uint(2)`,
			buf.ERR_SHORT,
		},
		{"bytes past the end", `r = buf.reader(b"\x01"); r.read_bytes(2)`, buf.ERR_SHORT},
		{"a str past the end", `r = buf.reader(b"\x01"); r.read_string(2)`, buf.ERR_SHORT},
		{
			"a negative count",
			`r = buf.reader(b"\x01"); r.read_bytes(-1)`,
			unpack.ERR_RANGE,
		},
		{"no width", `r = buf.reader(b"\x01"); r.read_uint(0)`, unpack.ERR_RANGE},
		{
			"an order nobody has",
			`r = buf.reader(b"\x01"); r.read_uint(1, "middle")`,
			integer.ERR_ORDER,
		},
		{"a number", `r = buf.reader(1)`, unpack.ERR_DATA},
	})
}

// TestReader_ReadsIntegersOfFixedWidth is each of the eight fixed widths, each
// taking as many bytes as it holds, and what a buffer wrote read back.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func TestReader_ReadsIntegersOfFixedWidth(t *testing.T) {
	_Check(t, []struct{ name, src, want string }{
		{
			"each unsigned width in turn",
			`r = buf.reader(b"\x01" * 15); out = [r.read_uint8(), r.read_uint16(), ` +
				`r.read_uint32(), r.read_uint64(), len(r)]`,
			`[1, 257, 16843009, 72340172838076673, 0]`,
		},
		{
			"uint16, little",
			`r = buf.reader(b"\x01\x02"); out = r.read_uint16("little")`,
			`513`,
		},
		{
			"uint64 at its top",
			`r = buf.reader(b"\xff" * 8); out = r.read_uint64()`,
			`18446744073709551615`,
		},
		{"int8 at its bottom", `r = buf.reader(b"\x80"); out = r.read_int8()`, `-128`},
		{
			"int16, little",
			`r = buf.reader(b"\xfe\xff"); out = r.read_int16("little")`,
			`-2`,
		},
		{"int32 of -1", `r = buf.reader(b"\xff" * 4); out = r.read_int32()`, `-1`},
		{
			"int64 at its bottom",
			`r = buf.reader(b"\x80" + b"\x00" * 7); out = r.read_int64()`,
			`-9223372036854775808`,
		},
		{
			"what a buffer wrote",
			`b = buf.new(); b.write_uint16(65535); b.write_int32(-5, "little"); ` +
				`b.write_string("hi"); r = buf.reader(b.bytes()); ` +
				`out = [r.read_uint16(), r.read_int32("little"), ` +
				`r.read_string(2)]`,
			`[65535, -5, "hi"]`,
		},
	})

	_Refuse(t, []struct {
		name string
		src  string
		want error
	}{
		{"past the end", `r = buf.reader(b"\x01"); r.read_uint16()`, buf.ERR_SHORT},
		{
			"an order nobody has",
			`r = buf.reader(b"\x01\x02"); r.read_uint16("middle")`,
			integer.ERR_ORDER,
		},
	})
}

// TestReader_StaysPutWhenARefusalStopsARead reads past the end and in an order
// nobody has, and expects the reader still at its first byte after each.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: through read_uint, where uint was
func TestReader_StaysPutWhenARefusalStopsARead(t *testing.T) {
	thread := &starlark.Thread{Name: SCRIPT}

	value, err := starlark.EvalOptions(
		dialect.OPTIONS,
		thread,
		SCRIPT,
		`buf.reader(b"\x01")`,
		_Environment(t),
	)
	if err != nil {
		t.Fatal(err)
	}

	reader, ok := value.(*buf.Reader)
	if !ok {
		t.Fatalf("buf.reader gave a %s", value.Type())
	}

	method, err := reader.Attr(buf.READ_UINT)
	if err != nil {
		t.Fatal(err)
	}

	refusals := []struct {
		name string
		args starlark.Tuple
		want error
	}{
		{"past the end", starlark.Tuple{starlark.MakeInt(2)}, buf.ERR_SHORT},
		{
			"an order nobody has",
			starlark.Tuple{starlark.MakeInt(1), starlark.String("middle")},
			integer.ERR_ORDER,
		},
	}

	for _, item := range refusals {
		_, err := starlark.Call(thread, method, item.args, nil)
		if !errors.Is(err, item.want) || reader.Len() != 1 {
			t.Fatalf(
				"%s gave %v with %d left, want %v with 1",
				item.name,
				err,
				reader.Len(),
				item.want,
			)
		}
	}
}

// TestFrozen_RefusesAChangeAndAllowsALook makes a buffer and a reader at the
// top of a script, which Starlark freezes once the script ends - as a spawn
// freezes what it hands over - and expects each change refused and each look
// allowed.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: the changes through the methods named for Go's
func TestFrozen_RefusesAChangeAndAllowsALook(t *testing.T) {
	thread := &starlark.Thread{Name: SCRIPT}
	env := _Environment(t)

	made, err := starlark.ExecFileOptions(dialect.OPTIONS, thread, SCRIPT, MADE, env)
	if err != nil {
		t.Fatal(err)
	}

	maps.Copy(env, made)

	changes := []string{
		`b.write_bytes("x")`,
		`b.write_string("x")`,
		`b.write_uint8(1)`,
		`b.write_int(1, 1)`,
		`r.read_bytes(1)`,
		`r.read_string(1)`,
		`r.read_uint8()`,
	}

	for _, change := range changes {
		_, err := starlark.EvalOptions(dialect.OPTIONS, thread, SCRIPT, change, env)
		if !errors.Is(err, buf.ERR_FROZEN) {
			t.Fatalf("%s gave %v, want %v", change, err, buf.ERR_FROZEN)
		}
	}

	looks := map[string]string{
		`b.bytes()`: `b"ab"`,
		`len(b)`:    `2`,
		`r[0]`:      `b"a"`,
		`len(r)`:    `2`,
	}

	for look, want := range looks {
		got, err := starlark.EvalOptions(dialect.OPTIONS, thread, SCRIPT, look, env)
		if err != nil || got.String() != want {
			t.Fatalf("%s gave %v, %v, want %s", look, got, err, want)
		}
	}
}
