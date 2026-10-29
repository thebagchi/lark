// Regression probes for the builtins that read their data through a view of
// the script's value rather than a copy: each gives what it gave when it read
// a copy, writes nothing into the data, and keeps nothing of it once it
// returns. The want of each row is the Go code that the builtin ran before it
// read a view, so a row that fails names a change in output.
package testing_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"strconv"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	// VIEW_SCRIPT is what a failure calls the expression it was given, and
	// VIEW_DATA the name each expression reads its data by.
	VIEW_SCRIPT = "view_test.star"
	VIEW_DATA   = "data"

	// AS_STRING and AS_BYTES name the two types a script hands data in as.
	AS_STRING = "string"
	AS_BYTES  = "bytes"

	// EVERY_BYTE is how many values a byte has. LARGE is past 64 KiB, so the
	// data is larger than any buffer a digest or an encoder holds.
	EVERY_BYTE = 256
	LARGE      = 1<<16 + 3

	// FLIP changes every bit of a byte, so a byte after the flip is never the
	// byte before it.
	FLIP = 0xff

	// HMAC_KEY and HMAC_MESSAGE are the argument of hmac that is not the data
	// under test, and SEED a checksum other than zero to continue from.
	HMAC_KEY     = "key"
	HMAC_MESSAGE = "message"
	SEED         = 12345

	// DECIMAL is the base Starlark writes an int in.
	DECIMAL = 10
)

// _Reader is one builtin call that reads data through a view, and the Go code
// that gives what the call gave when it read a copy.
type _Reader struct {
	expression string
	want       func(data []byte) (string, error)
}

// _Input is one piece of data that each reader reads.
type _Input struct {
	name string
	data []byte
}

// TestView_ReadersGiveWhatTheyGave reads each input, as a string and as bytes,
// through each builtin that reads a view. The output is the output of the copy
// it replaced. The data is the same after the call, so the builtin wrote
// nothing into a value the script still holds. The output is the same after
// the data changes, so the builtin kept nothing of it.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func TestView_ReadersGiveWhatTheyGave(t *testing.T) {
	for _, reader := range _Readers() {
		for _, input := range _Inputs() {
			for _, kind := range []string{
				AS_STRING,
				AS_BYTES,
			} {
				t.Run(
					reader.expression+"/"+input.name+"/"+kind,
					func(t *testing.T) {
						_CheckReader(
							t,
							reader,
							input,
							kind,
						)
					},
				)
			}
		}
	}
}

// TestView_RefusesAsBefore records the refusals of the readers, with the text
// each gave when it read a copy, so the view refuses what the copy refused.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func TestView_RefusesAsBefore(t *testing.T) {
	cases := []struct {
		expression string
		want       string
	}{
		{
			`hex.from_bytes(1)`,
			"hex.from_bytes got int: wants bytes or a string",
		},
		{
			`hex.from_bytes()`,
			"hex.from_bytes: got 0 arguments, want 1",
		},
		{
			`hex.from_bytes("a", "b")`,
			"hex.from_bytes: got 2 arguments, want 1",
		},
		{
			`base64.encode(None)`,
			"base64.encode got NoneType: wants bytes or a string",
		},
		{
			`base64.urlencode([])`,
			"base64.urlencode got list: wants bytes or a string",
		},
		{
			`base64.encode()`,
			"base64.encode: got 0 arguments, want 1",
		},
		{
			`hash.sha256(1)`,
			"hash.sha256 got int: wants bytes or a string",
		},
		{
			`hash.md5()`,
			"hash.md5: got 0 arguments, want 1",
		},
		{
			`hash.hmac("sha256", 1, "d")`,
			"hash.hmac got int: wants bytes or a string",
		},
		{
			`hash.hmac("sha256", "k", 1)`,
			"hash.hmac got int: wants bytes or a string",
		},
		{
			`hash.crc32(1)`,
			"hash.crc32 got int: wants bytes or a string",
		},
		{
			`hash.crc32(b"", -1)`,
			"hash.crc32 got -1 to continue: out of range",
		},
	}

	for _, item := range cases {
		t.Run(
			item.expression,
			func(t *testing.T) {
				_, err := _EvalData(
					t,
					item.expression,
					starlark.None,
				)
				if err == nil || err.Error() != item.want {
					t.Fatalf(
						"%s gave %v, want %q",
						item.expression,
						err,
						item.want,
					)
				}
			},
		)
	}
}

// _CheckReader reads one input of one kind through one reader, over bytes
// that this test owns, so that a write through them shows what the call kept.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _CheckReader(
	t *testing.T,
	reader *_Reader,
	input *_Input,
	kind string,
) {
	t.Helper()

	var (
		buf  = bytes.Clone(input.data)
		view = _View(
			kind,
			buf,
		)
	)

	got, err := _EvalData(
		t,
		reader.expression,
		view,
	)
	if err != nil {
		t.Fatalf(
			"%s: %v",
			reader.expression,
			err,
		)
	}

	want, err := reader.want(input.data)
	if err != nil {
		t.Fatalf(
			"%s: the want: %v",
			reader.expression,
			err,
		)
	}

	if _Text(got) != want {
		t.Fatalf(
			"%s gave %.80s, want %.80s",
			reader.expression,
			_Text(got),
			want,
		)
	}

	if !bytes.Equal(
		buf,
		input.data,
	) {
		t.Fatalf(
			"%s wrote into its data",
			reader.expression,
		)
	}

	for idx := range buf {
		buf[idx] ^= FLIP
	}

	if _Text(got) != want {
		t.Fatalf(
			"%s gave an output that changed with its data",
			reader.expression,
		)
	}
}

// _Readers is each builtin that reads data through a view, with the code it
// ran before. A digest is upper-case hex, as fmt's %X wrote it.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _Readers() []*_Reader {
	return []*_Reader{
		{
			expression: `hex.from_bytes(data)`,
			want: func(data []byte) (string, error) {
				return fmt.Sprintf(
					"%X",
					data,
				), nil
			},
		},
		{
			expression: `base64.encode(data)`,
			want: func(data []byte) (string, error) {
				return base64.StdEncoding.EncodeToString(data), nil
			},
		},
		{
			expression: `base64.urlencode(data)`,
			want: func(data []byte) (string, error) {
				return base64.RawURLEncoding.EncodeToString(data), nil
			},
		},
		{
			expression: `hash.md5(data)`,
			want: func(data []byte) (string, error) {
				return fmt.Sprintf(
					"%X",
					md5.Sum(data),
				), nil
			},
		},
		{
			expression: `hash.sha1(data)`,
			want: func(data []byte) (string, error) {
				return fmt.Sprintf(
					"%X",
					sha1.Sum(data),
				), nil
			},
		},
		{
			expression: `hash.sha256(data)`,
			want: func(data []byte) (string, error) {
				return fmt.Sprintf(
					"%X",
					sha256.Sum256(data),
				), nil
			},
		},
		{
			expression: `hash.sha512(data)`,
			want: func(data []byte) (string, error) {
				return fmt.Sprintf(
					"%X",
					sha512.Sum512(data),
				), nil
			},
		},
		{
			expression: `hash.hmac("sha256", data, "` + HMAC_MESSAGE + `")`,
			want: func(data []byte) (string, error) {
				return _GoHMACOf(
					data,
					[]byte(HMAC_MESSAGE),
				)
			},
		},
		{
			expression: `hash.hmac("sha256", "` + HMAC_KEY + `", data)`,
			want: func(data []byte) (string, error) {
				return _GoHMACOf(
					[]byte(HMAC_KEY),
					data,
				)
			},
		},
		{
			expression: `hash.crc32(data)`,
			want: func(data []byte) (string, error) {
				return strconv.FormatUint(
					uint64(crc32.ChecksumIEEE(data)),
					DECIMAL,
				), nil
			},
		},
		{
			expression: `hash.crc32(data, ` + strconv.Itoa(SEED) + `)`,
			want: func(data []byte) (string, error) {
				return strconv.FormatUint(
					uint64(crc32.Update(
						SEED,
						crc32.IEEETable,
						data,
					)),
					DECIMAL,
				), nil
			},
		},
	}
}

// _Inputs is the data each reader reads: none, text, bytes that are not text,
// every byte value once, and more than any buffer of a reader holds.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _Inputs() []*_Input {
	return []*_Input{
		{
			name: "empty",
			data: []byte{},
		},
		{
			name: "ascii",
			data: []byte("Hello, World"),
		},
		{
			name: "binary",
			data: []byte{
				0x00,
				0x01,
				0x0a,
				0x7f,
				0x80,
				0xfe,
				0xff,
			},
		},
		{
			name: "every byte",
			data: _Cycle(EVERY_BYTE),
		},
		{
			name: "large",
			data: _Cycle(LARGE),
		},
	}
}

// _Cycle is size bytes that run through every byte value in turn.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _Cycle(size int) []byte {
	data := make(
		[]byte,
		size,
	)

	for idx := range data {
		data[idx] = byte(idx)
	}

	return data
}

// _View is buf as a script value of kind, over the same bytes with no copy.
// A string over bytes that change breaks the rule of unsafe.String; this test
// breaks it on purpose, because only a write shows what a call kept.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _View(
	kind string,
	buf []byte,
) starlark.Value {
	text := unsafe.String(
		unsafe.SliceData(buf),
		len(buf),
	)

	if kind == AS_STRING {
		return starlark.String(text)
	}

	return starlark.Bytes(text)
}

// _EvalData evaluates one expression against the default environment, with
// data bound to VIEW_DATA.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _EvalData(
	t *testing.T,
	expression string,
	data starlark.Value,
) (starlark.Value, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.DEFAULT)
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	env[VIEW_DATA] = data

	return starlark.EvalOptions(
		dialect.OPTIONS,
		&starlark.Thread{
			Name: VIEW_SCRIPT,
		},
		VIEW_SCRIPT,
		expression,
		env,
	)
}

// _Text is a value as text to compare: a string as its characters, and
// anything else as Starlark writes it.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _Text(value starlark.Value) string {
	text, ok := value.(starlark.String)
	if ok {
		return string(text)
	}

	return value.String()
}

// _GoHMACOf is HMAC-SHA256 of message under key, as upper-case hex. The error
// of the write is returned, not dropped, though a digest never gives one.
//
// Revisions:
//   - 2026-10-29 01:07: initial creation
func _GoHMACOf(
	key []byte,
	message []byte,
) (string, error) {
	made := hmac.New(
		sha256.New,
		key,
	)

	if _, err := made.Write(message); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"%X",
		made.Sum(nil),
	), nil
}
