package tests

import (
	"strconv"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Viewed is a bytes value over memory the test owns, and that memory. A
// builtin that kept a view of the value would show a later change of the
// memory.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Viewed(size int) (starlark.Bytes, []byte) {
	owned := _Data(size)

	return starlark.Bytes(unsafe.String(
		unsafe.SliceData(owned),
		len(owned),
	)), owned
}

// TestFromBytes_MakesNoCopyOfItsInput counts the allocations of
// binary.from_bytes on 1 KiB through the builtin. The two are the bits and the
// boxing of the result. A copy of the input is a third.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b, plan D4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromBytes_MakesNoCopyOfItsInput(t *testing.T) {
	const MOST = 2

	var (
		fn = _Builtin(
			t,
			spec.FROM_BYTES,
		)
		args   = starlark.Tuple{starlark.Bytes(_Data(1024))}
		thread = &starlark.Thread{
			Name: BENCH,
		}
	)

	allocs := testing.AllocsPerRun(
		50,
		func() {
			_, err := fn.CallInternal(
				thread,
				args,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
		},
	)

	if allocs > MOST {
		t.Fatalf(
			"from_bytes makes %v allocations, want at most %d",
			allocs,
			MOST,
		)
	}
}

// TestFromBytes_KeepsNoViewOfItsInput proves the view ends with the call. The
// input is a bytes value over memory the test owns. The test calls
// from_bytes, then changes that memory. The result must keep its bits, and
// must not share memory with the input. A result that was the view, or a part
// of it, would fail one of the two checks.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b, plan D4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromBytes_KeepsNoViewOfItsInput(t *testing.T) {
	fn := _Builtin(
		t,
		spec.FROM_BYTES,
	)

	for _, size := range []int{0, 1, 7, 256, 1024} {
		t.Run(
			strconv.Itoa(size),
			func(t *testing.T) {
				var (
					input, owned = _Viewed(size)
					want         = binary.Encode(_Data(size))
				)

				value, err := fn.CallInternal(
					&starlark.Thread{
						Name: BENCH,
					},
					starlark.Tuple{input},
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				for idx := range owned {
					owned[idx] ^= 0xff
				}

				got, _ := starlark.AsString(value)
				if got != want {
					t.Fatalf(
						"result changed: got %d bytes, want %d",
						len(got),
						len(want),
					)
				}

				if size > 0 && _Shares(
					got,
					string(input),
				) {
					t.Fatal("result shares memory with the input")
				}
			},
		)
	}
}

// TestFromBytes_ReadsBytesAndStrAlike pins the output for a bytes value and a
// str of the same octets, and for the refusals: the same bits as before the
// view, and the same refusal text.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b, plan D4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromBytes_ReadsBytesAndStrAlike(t *testing.T) {
	fn := _Builtin(
		t,
		spec.FROM_BYTES,
	)

	for _, item := range []struct {
		name string
		in   starlark.Value
		want string
	}{
		{
			"empty bytes",
			starlark.Bytes(""),
			"",
		},
		{
			"empty str",
			starlark.String(""),
			"",
		},
		{
			"bytes",
			starlark.Bytes("\x00\xff\x05"),
			"000000001111111100000101",
		},
		{
			"str",
			starlark.String("A"),
			"01000001",
		},
		{
			"str multibyte",
			starlark.String("é"),
			"1100001110101001",
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				value, err := fn.CallInternal(
					&starlark.Thread{
						Name: BENCH,
					},
					starlark.Tuple{item.in},
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				if got, _ := starlark.AsString(value); got != item.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						item.want,
					)
				}
			},
		)
	}

	for _, item := range []struct {
		name   string
		args   starlark.Tuple
		kwargs []starlark.Tuple
		want   string
	}{
		{
			"int",
			starlark.Tuple{starlark.MakeInt(1)},
			nil,
			"binary.from_bytes got int: " + unpack.ErrData.Error(),
		},
		{
			"none",
			starlark.Tuple{},
			nil,
			"binary.from_bytes: got 0 arguments, want 1",
		},
		{
			"keyword",
			starlark.Tuple{},
			[]starlark.Tuple{{starlark.String("data"), starlark.Bytes("a")}},
			"binary.from_bytes: unexpected keyword arguments",
		},
	} {
		t.Run(
			"refuses "+item.name,
			func(t *testing.T) {
				_, err := fn.CallInternal(
					&starlark.Thread{
						Name: BENCH,
					},
					item.args,
					item.kwargs,
				)
				if err == nil || err.Error() != item.want {
					t.Fatalf(
						"got %v, want %q",
						err,
						item.want,
					)
				}
			},
		)
	}
}

// _Shares reports whether two texts start at one address.
//
// Revisions:
//   - 2026-11-03 22:28: initial creation (C38b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Shares(
	left string,
	right string,
) bool {
	return unsafe.StringData(left) == unsafe.StringData(right)
}
