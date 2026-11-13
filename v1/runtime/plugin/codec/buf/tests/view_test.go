package tests

import (
	"strconv"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// THREAD names the thread that each call here runs on.
	//
	// Revisions:
	//   - 2026-11-04 00:15: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	THREAD = "view"
)

var (
	// SIZES are the counts of bytes that the benchmark writes.
	//
	// Revisions:
	//   - 2026-11-04 00:15: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SIZES = []int{
		16,
		1024,
		65536,
	}
)

// _Data is size bytes that run through every byte value.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
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

// _Viewed is a bytes value over memory the test owns, and that memory. A
// method that kept a view of the value would show a later change of the memory.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Viewed(size int) (starlark.Bytes, []byte) {
	owned := _Data(size)

	return starlark.Bytes(unsafe.String(
		unsafe.SliceData(owned),
		len(owned),
	)), owned
}

// _Method makes a new buffer and returns its method name and its bytes method,
// each bound to it, as a script reads them.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Method(
	tb testing.TB,
	name string,
) (*starlark.Builtin, *starlark.Builtin) {
	tb.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		tb.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env["buf"].(*starlarkstruct.Module)
	if !ok {
		tb.Fatalf(
			"%s is not a module",
			"buf",
		)
	}

	maker, ok := module.Members["new"].(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"%s.%s is not a builtin",
			"buf",
			"new",
		)
	}

	made, err := maker.CallInternal(
		&starlark.Thread{
			Name: THREAD,
		},
		nil,
		nil,
	)
	if err != nil {
		tb.Fatal(err)
	}

	attrs := made.(starlark.HasAttrs)

	return _Bound(
			tb,
			attrs,
			name,
		), _Bound(
			tb,
			attrs,
			"bytes",
		)
}

// _Bound is the method name of a buffer.
//
// Revisions:
//   - 2026-11-03 22:18: initial creation (C38b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Bound(
	tb testing.TB,
	attrs starlark.HasAttrs,
	name string,
) *starlark.Builtin {
	tb.Helper()

	value, err := attrs.Attr(name)
	if err != nil {
		tb.Fatal(err)
	}

	fn, ok := value.(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"%s is not a builtin",
			name,
		)
	}

	return fn
}

// BenchmarkWriteBytes measures buf's write_bytes on bytes of each size,
// through the method, on a new buffer for each call, so the copy of the
// argument shows in the allocations. The buffers are made in batches with the
// timer stopped, so the count holds the calls only. The bytes per second are
// bytes of data.
//
// Revisions:
//   - 2026-11-03 22:22: initial creation (C38b, plan D4)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkWriteBytes(b *testing.B) {
	const BATCH = 64

	for _, size := range SIZES {
		b.Run(
			strconv.Itoa(size),
			func(b *testing.B) {
				var (
					args = starlark.Tuple{
						starlark.Bytes(_Data(size)),
					}
					thread = &starlark.Thread{
						Name: THREAD,
					}
					writes = make(
						[]*starlark.Builtin,
						BATCH,
					)
				)

				b.SetBytes(int64(size))
				b.ReportAllocs()

				for done := 0; done < b.N; done += BATCH {
					b.StopTimer()

					for idx := range writes {
						writes[idx], _ = _Method(
							b,
							"write_bytes",
						)
					}

					b.StartTimer()

					for idx := 0; idx < BATCH && done+idx < b.N; idx++ {
						_, err := writes[idx].CallInternal(
							thread,
							args,
							nil,
						)
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			},
		)
	}
}

// TestWriteBytes_MakesNoCopyOfItsInput counts the allocations of write_bytes
// on 1 KiB into a new buffer, through the method. The one is the growth of the
// buffer to hold the data. A copy of the input is a second. The buffers are
// made before the count, so the count holds the call only.
//
// Revisions:
//   - 2026-11-03 22:19: initial creation (C38b, plan D4)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWriteBytes_MakesNoCopyOfItsInput(t *testing.T) {
	const (
		MOST = 1
		RUNS = 50
	)

	var (
		writes = make(
			[]*starlark.Builtin,
			0,
			RUNS+1,
		)
		args   = starlark.Tuple{starlark.Bytes(_Data(1024))}
		thread = &starlark.Thread{
			Name: THREAD,
		}
		next = 0
	)

	// AllocsPerRun calls once to warm up, then runs times.
	for range RUNS + 1 {
		write, _ := _Method(
			t,
			"write_bytes",
		)

		writes = append(
			writes,
			write,
		)
	}

	allocs := testing.AllocsPerRun(
		RUNS,
		func() {
			_, err := writes[next].CallInternal(
				thread,
				args,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}

			next++
		},
	)

	if allocs > MOST {
		t.Fatalf(
			"write_bytes makes %v allocations, want at most %d",
			allocs,
			MOST,
		)
	}
}

// TestWriteBytes_KeepsNoViewOfItsInput proves the view ends with the call. The
// input is a bytes value over memory the test owns. The test writes it, then
// changes that memory. The buffer must keep the bytes it was given, and must
// not share memory with the input. A buffer that kept the view, in place of
// appending a copy, would fail both checks.
//
// Revisions:
//   - 2026-11-03 22:19: initial creation (C38b, plan D4)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWriteBytes_KeepsNoViewOfItsInput(t *testing.T) {
	for _, size := range []int{0, 1, 7, 256, 1024} {
		t.Run(
			strconv.Itoa(size),
			func(t *testing.T) {
				var (
					write, read = _Method(
						t,
						"write_bytes",
					)
					input, own = _Viewed(size)
					thread     = &starlark.Thread{
						Name: THREAD,
					}
				)

				_, err := write.CallInternal(
					thread,
					starlark.Tuple{input},
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				for idx := range own {
					own[idx] ^= 0xff
				}

				got, err := read.CallInternal(
					thread,
					nil,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				text := string(got.(starlark.Bytes))
				if text != string(_Data(size)) {
					t.Fatalf(
						"buffer changed with its input: %d bytes",
						len(text),
					)
				}

				if size > 0 && _Shares(
					text,
					string(input),
				) {
					t.Fatal("buffer shares memory with the input")
				}
			},
		)
	}
}

// TestWriteBytes_WritesBytesAndStrAlike pins the contents after writes of
// bytes and of a str, and the refusals: the same bytes as before the view,
// and the same text for each refusal.
//
// Revisions:
//   - 2026-11-03 22:19: initial creation (C38b, plan D4)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWriteBytes_WritesBytesAndStrAlike(t *testing.T) {
	for _, item := range []struct {
		name string
		in   []starlark.Value
		want string
	}{
		{
			"nothing written",
			nil,
			"",
		},
		{
			"empty",
			[]starlark.Value{starlark.Bytes("")},
			"",
		},
		{
			"bytes",
			[]starlark.Value{starlark.Bytes("\x00\xff")},
			"\x00\xff",
		},
		{
			"str",
			[]starlark.Value{starlark.String("\u00e9")},
			"\xc3\xa9",
		},
		{
			"two writes",
			[]starlark.Value{starlark.Bytes("ab"), starlark.String("cd")},
			"abcd",
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					write, read = _Method(
						t,
						"write_bytes",
					)
					thread = &starlark.Thread{
						Name: THREAD,
					}
				)

				for _, one := range item.in {
					_, err := write.CallInternal(
						thread,
						starlark.Tuple{one},
						nil,
					)
					if err != nil {
						t.Fatal(err)
					}
				}

				got, err := read.CallInternal(
					thread,
					nil,
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				if text := string(got.(starlark.Bytes)); text != item.want {
					t.Fatalf(
						"got %q, want %q",
						text,
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
			"write_bytes got int: " + unpack.ErrData.Error(),
		},
		{
			"none",
			starlark.Tuple{},
			nil,
			"write_bytes: got 0 arguments, want 1",
		},
	} {
		t.Run(
			"refuses "+item.name,
			func(t *testing.T) {
				var write, _ = _Method(
					t,
					"write_bytes",
				)

				_, err := write.CallInternal(
					&starlark.Thread{
						Name: THREAD,
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
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Shares(
	left string,
	right string,
) bool {
	return unsafe.StringData(left) == unsafe.StringData(right)
}
