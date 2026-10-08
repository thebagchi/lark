// Probes for the codec modules, buf, and the arity check, as the developer
// asked in testing.md on 2026-10-08.
package testing_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	codechex "github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	"github.com/thebagchi/lark/v1/runtime/plugin/event"
	"github.com/thebagchi/lark/v1/runtime/plugin/state"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _MEGABYTE is what lark -m 1 gives a run.
const _MEGABYTE = 1 << 20

// _Value compiles src and runs it. A compile error comes back as the error, so
// a caller can tell a refusal at compile from a refusal at run.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func _Value(t *testing.T, src string, opts ...runtime.RunOption) (starlark.Value, error) {
	t.Helper()

	built, err := runtime.Compile(&runtime.Source{
		Entry: "probe.star",
		Text:  []byte(src),
	})
	if err != nil {
		return nil, err
	}

	return runtime.Start(t.Context(), built, opts...).Wait()
}

// _Loaded compiles main, which loads lib.star from the same directory, and
// runs it.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func _Loaded(t *testing.T, lib string, main string) (starlark.Value, error) {
	t.Helper()

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "lib.star"), []byte(lib), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	built, err := runtime.Compile(&runtime.Source{
		Entry: filepath.Join(dir, "main.star"),
		Text:  []byte(main),
	})
	if err != nil {
		return nil, err
	}

	return runtime.Start(t.Context(), built).Wait()
}

// TestArity_AnOperationIsOneArgument checks the operations the arity note
// names. Each is one positional argument. A keyword-only parameter, **kwargs
// and a loaded function are in the same table.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestArity_AnOperationIsOneArgument(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
		fail error
	}{
		{
			name: "equal",
			src:  "def f(x):\n  return x\ndef main():\n  return f(1 == 1)\n",
			want: "True",
		},
		{
			name: "in",
			src:  "def f(x):\n  return x\ndef main():\n  return f(1 in [1])\n",
			want: "True",
		},
		{
			name: "and",
			src:  "def f(x):\n  return x\ndef main():\n  return f(True and False)\n",
			want: "False",
		},
		{
			name: "or",
			src:  "def f(x):\n  return x\ndef main():\n  return f(False or 1)\n",
			want: "1",
		},
		{
			name: "remainder",
			src:  "def f(x):\n  return x\ndef main():\n  return f(5 % 2)\n",
			want: "1",
		},
		{
			name: "slice",
			src:  "def f(x):\n  return x\ndef main():\n  return f([1, 2, 3][1:])\n",
			want: "[2, 3]",
		},
		{
			name: "conditional",
			src:  "def f(x):\n  return x\ndef main():\n  return f(1 if True else 0)\n",
			want: "1",
		},
		{
			name: "call",
			src: "def g():\n  return 7\n" +
				"def f(x):\n  return x\n" +
				"def main():\n  return f(g())\n",
			want: "7",
		},
		{
			name: "keyword only, named",
			src: "def only(*, x):\n  return x\n" +
				"def main():\n  return only(x = 1 + 2)\n",
			want: "3",
		},
		{
			name: "keyword only, by position",
			src:  "def only(*, x):\n  return x\ndef main():\n  return only(1 + 2)\n",
			fail: runtime.ERR_ARITY,
		},
		{
			name: "kwargs takes a named operation",
			src: "def k(x, **more):\n  return more[\"y\"]\n" +
				"def main():\n  return k(1, y = 1 + 2)\n",
			want: "3",
		},
		{
			name: "kwargs does not take a second position",
			src:  "def k(x, **more):\n  pass\ndef main():\n  n = 1\n  k(1, n + 1)\n",
			fail: runtime.ERR_ARITY,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src)
			if item.fail != nil {
				if !errors.Is(err, item.fail) {
					t.Fatalf("got %v, want %v", err, item.fail)
				}

				return
			}

			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != item.want {
				t.Fatalf("got %s, want %s", got.String(), item.want)
			}
		})
	}
}

// TestArity_ALoadedFunctionTakesAnOperation checks a function defined in
// another file. An operation is one argument. A call with nothing is refused.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestArity_ALoadedFunctionTakesAnOperation(t *testing.T) {
	t.Parallel()

	const lib = "def g(x):\n  return x\n"

	cases := []struct {
		name string
		main string
		want string
		fail error
	}{
		{
			name: "an addition",
			main: "load(\"lib.star\", \"g\")\ndef main():\n  return g(1 + 2)\n",
			want: "3",
		},
		{
			name: "a comparison",
			main: "load(\"lib.star\", \"g\")\ndef main():\n  return g(1 == 2)\n",
			want: "False",
		},
		{
			name: "nothing",
			main: "load(\"lib.star\", \"g\")\ndef main():\n  g()\n",
			fail: runtime.ERR_ARITY,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Loaded(t, lib, item.main)
			if item.fail != nil {
				if !errors.Is(err, item.fail) {
					t.Fatalf("got %v, want %v", err, item.fail)
				}

				return
			}

			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != item.want {
				t.Fatalf("got %s, want %s", got.String(), item.want)
			}
		})
	}
}

// TestBuf_TheEndsOfAnInteger checks the ends the note names, written with the
// powers a script writes, and a width of zero, below zero, and past the run.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestBuf_TheEndsOfAnInteger(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
		fail error
	}{
		{
			name: "uint64 at its top",
			src: `
def main():
    b = buf.new()
    b.write_uint64(18446744073709551615)
    return hex.from_bytes(b.bytes())
`,
			want: `"FFFFFFFFFFFFFFFF"`,
		},
		{
			name: "uint64 one past",
			src: `
def main():
    b = buf.new()
    b.write_uint64(18446744073709551616)
`,
			fail: unpack.ERR_RANGE,
		},
		{
			name: "int64 at its top",
			src: `
def main():
    b = buf.new()
    b.write_int64(9223372036854775807)
    return hex.from_bytes(b.bytes())
`,
			want: `"7FFFFFFFFFFFFFFF"`,
		},
		{
			name: "int64 one past its top",
			src: `
def main():
    b = buf.new()
    b.write_int64(9223372036854775808)
`,
			fail: unpack.ERR_RANGE,
		},
		{
			name: "int64 at its bottom",
			src: `
def main():
    b = buf.new()
    b.write_int64(-9223372036854775808)
    return hex.from_bytes(b.bytes())
`,
			want: `"8000000000000000"`,
		},
		{
			name: "int64 one past its bottom",
			src: `
def main():
    b = buf.new()
    b.write_int64(-9223372036854775809)
`,
			fail: unpack.ERR_RANGE,
		},
		{
			name: "a width of zero",
			src: `
def main():
    b = buf.new()
    b.write_uint(1, 0)
`,
			fail: unpack.ERR_RANGE,
		},
		{
			name: "a width below zero",
			src: `
def main():
    b = buf.new()
    b.write_int(1, -1)
`,
			fail: unpack.ERR_RANGE,
		},
		{
			name: "a width past the run",
			src: `
def main():
    b = buf.new()
    b.write_uint(0, 1 << 40)
`,
			fail: runtime.ERR_MEMORY,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src)
			if item.fail != nil {
				if !errors.Is(err, item.fail) {
					t.Fatalf("got %v, want %v", err, item.fail)
				}

				return
			}

			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != item.want {
				t.Fatalf("got %s, want %s", got.String(), item.want)
			}
		})
	}
}

// TestBuf_AcrossThreads checks the three shapes the note names. A container
// that holds a buffer is not data. A reader handed to a spawn cannot be read.
// A buffer a thread makes and returns can still be written by the caller.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestBuf_AcrossThreads(t *testing.T) {
	t.Parallel()

	t.Run("a list in the store", func(t *testing.T) {
		t.Parallel()

		_, err := _Value(t, `
def main():
    state.set("k", [buf.new()])
`)
		if !errors.Is(err, state.ERR_NOT_DATA) {
			t.Fatalf("got %v, want %v", err, state.ERR_NOT_DATA)
		}
	})

	t.Run("a dict in the store", func(t *testing.T) {
		t.Parallel()

		_, err := _Value(t, `
def main():
    state.set("k", {"a": buf.new()})
`)
		if !errors.Is(err, state.ERR_NOT_DATA) {
			t.Fatalf("got %v, want %v", err, state.ERR_NOT_DATA)
		}
	})

	t.Run("a post of a list", func(t *testing.T) {
		t.Parallel()

		_, err := _Value(t, `
def main():
    event.post("k", [buf.reader(b"ab")])
`)
		if !errors.Is(err, event.ERR_NOT_DATA) {
			t.Fatalf("got %v, want %v", err, event.ERR_NOT_DATA)
		}
	})

	t.Run("a reader handed to a spawn", func(t *testing.T) {
		t.Parallel()

		_, err := _Value(t, `
def main():
    reader = buf.reader(b"abcd")
    join(spawn(lambda: reader.read_bytes(1)))
`)
		if !errors.Is(err, buf.ERR_FROZEN) {
			t.Fatalf("got %v, want %v", err, buf.ERR_FROZEN)
		}
	})

	t.Run("a buffer a thread returns", func(t *testing.T) {
		t.Parallel()

		got, err := _Value(t, `
def child():
    buffer = buf.new()
    buffer.write_bytes("ab")
    return buffer

def main():
    buffer = join(spawn(child))[0]
    buffer.write_bytes("c")
    return buffer.bytes()
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}

		if got.String() != `b"abc"` {
			t.Fatalf("got %s, want b\"abc\"", got.String())
		}
	})
}

// TestHex_APrefix checks the prefix shapes the note names, and the same shapes
// for bits.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestHex_APrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
		fail error
	}{
		{
			name: "0x alone is no bytes",
			src:  "def main():\n  return hex.to_bytes(\"0x\")\n",
			want: `b""`,
		},
		{
			name: "0X alone is no bytes",
			src:  "def main():\n  return hex.to_bytes(\"0X\")\n",
			want: `b""`,
		},
		{
			name: "0x alone is not an integer",
			src:  "def main():\n  return hex.to_int(\"0x\")\n",
			fail: codechex.ERR_HEX,
		},
		{
			name: "0X alone is not an integer",
			src:  "def main():\n  return hex.to_int(\"0X\")\n",
			fail: codechex.ERR_HEX,
		},
		{
			name: "a second prefix stays",
			src:  "def main():\n  return hex.to_bytes(\"0X0x41\")\n",
			fail: codechex.ERR_HEX,
		},
		{
			name: "odd digits after a prefix",
			src:  "def main():\n  return hex.to_bytes(\"0xabc\")\n",
			fail: codechex.ERR_HEX,
		},
		{
			name: "a space before a prefix",
			src:  "def main():\n  return hex.to_bytes(\" 0x41\")\n",
			fail: codechex.ERR_HEX,
		},
		{
			name: "a space before a bit prefix",
			src:  "def main():\n  return binary.to_int(\" 0b101\")\n",
			fail: binary.ERR_BITS,
		},
		{
			name: "0x is read",
			src:  "def main():\n  return hex.to_bytes(\"0x4142\")\n",
			want: `b"AB"`,
		},
		{
			name: "0X is read",
			src:  "def main():\n  return hex.to_int(\"0X12c\")\n",
			want: "300",
		},
		{
			name: "0b alone is no bytes",
			src:  "def main():\n  return binary.to_bytes(\"0b\")\n",
			want: `b""`,
		},
		{
			name: "0b alone is not an integer",
			src:  "def main():\n  return binary.to_int(\"0b\")\n",
			fail: binary.ERR_BITS,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src)
			if item.fail != nil {
				if !errors.Is(err, item.fail) {
					t.Fatalf("got %v, want %v", err, item.fail)
				}

				return
			}

			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != item.want {
				t.Fatalf("got %s, want %s", got.String(), item.want)
			}
		})
	}
}

// TestBudget_OneMegabyte checks the three calls the note names under the
// ceiling lark -m 1 sets. A small width runs. A width of one megabyte runs
// too: the spine is not charged, so the whole ceiling is free. One byte more
// is refused, and the message names that ceiling.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestBudget_OneMegabyte(t *testing.T) {
	t.Parallel()

	small := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "integer.to_bytes",
			src: `
def main():
    return hex.from_bytes(integer.to_bytes(256, 2))
`,
			want: `"0100"`,
		},
		{
			name: "hex.from_int",
			src:  "def main():\n  return hex.from_int(300, 4)\n",
			want: `"012C"`,
		},
		{
			name: "a write",
			src: `
def main():
    b = buf.new()
    b.write_uint16(258)
    return hex.from_bytes(b.bytes())
`,
			want: `"0102"`,
		},
	}

	for _, item := range small {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src, runtime.WithMemory(_MEGABYTE))
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != item.want {
				t.Fatalf("got %s, want %s", got.String(), item.want)
			}
		})
	}

	whole := []struct {
		name string
		src  string
	}{
		{
			name: "integer.to_bytes of the whole ceiling",
			src:  "def main():\n  return len(integer.to_bytes(0, 1 << 20))\n",
		},
		{
			name: "a write of the whole ceiling",
			src: `
def main():
    b = buf.new()
    b.write_uint(0, 1 << 20)
    return len(b)
`,
		},
	}

	for _, item := range whole {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src, runtime.WithMemory(_MEGABYTE))
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			if got.String() != "1048576" {
				t.Fatalf("got %s, want the whole ceiling", got.String())
			}
		})
	}

	wide := []struct {
		name string
		src  string
	}{
		{
			name: "integer.to_bytes past the ceiling",
			src:  "def main():\n  integer.to_bytes(1, (1 << 20) + 1)\n",
		},
		{
			name: "hex.from_int past the ceiling",
			src:  "def main():\n  hex.from_int(0, (1 << 20) + 1)\n",
		},
		{
			name: "a write past the ceiling",
			src: `
def main():
    b = buf.new()
    b.write_uint(0, (1 << 20) + 1)
`,
		},
	}

	for _, item := range wide {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			_, err := _Value(t, item.src, runtime.WithMemory(_MEGABYTE))
			if err == nil || !errors.Is(err, runtime.ERR_MEMORY) {
				t.Fatalf("got %v, want the one megabyte ceiling", err)
			}

			if !strings.Contains(err.Error(), "1048576 of 1048576") {
				t.Fatalf("got %v, want the ceiling named", err)
			}
		})
	}
}

// TestFromInt_AWideWidthIsTheDigits checks a width Go's fmt will not use as a
// field width. The call charges that width, then returns a format complaint
// as the digits.
//
// Revisions:
//   - 2026-10-08 22:10: initial creation
func TestFromInt_AWideWidthIsTheDigits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		src   string
		width int
	}{
		{
			name:  "hex, one past what fmt allows",
			src:   "def main():\n  return hex.from_int(0, 1000001)\n",
			width: 1000001,
		},
		{
			name:  "hex, one megabyte of digits",
			src:   "def main():\n  return hex.from_int(0, 1 << 20)\n",
			width: 1 << 20,
		},
		{
			name:  "binary, one past what fmt allows",
			src:   "def main():\n  return binary.from_int(0, 1000001)\n",
			width: 1000001,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			t.Parallel()

			got, err := _Value(t, item.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			text, ok := got.(starlark.String)
			if !ok || len(text) != item.width {
				shown := got.String()
				if len(shown) > 80 {
					shown = shown[:80]
				}

				t.Fatalf("got %s, want %d digits", shown, item.width)
			}
		})
	}
}

// TestCodec_AFlatNameIsUndefined checks a name from the old codec module. A
// digest is upper case.
//
// Revisions:
//   - 2026-10-08 21:55: initial creation
func TestCodec_AFlatNameIsUndefined(t *testing.T) {
	t.Parallel()

	_, err := _Value(t, "def main():\n  return bytes2hex(b\"A\")\n")
	if err == nil || !strings.Contains(err.Error(), "undefined: bytes2hex") {
		t.Fatalf("got %v, want bytes2hex undefined", err)
	}

	sum := sha256.Sum256([]byte("abc"))
	want := strings.ToUpper(hex.EncodeToString(sum[:]))

	got, err := _Value(t, "def main():\n  return hash.sha256(\"abc\")\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if got.String() != `"`+want+`"` {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
}
