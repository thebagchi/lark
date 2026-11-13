package tests

import (
	"testing"
)

// TestBase64_KeepsTheNameAndTheTextOfEachBuiltin pins the output and the whole
// error text of the four builtins, for the same expression as a script writes
// it. A refactor that changed a name, an alphabet, or one word of an error
// fails here.
//
// Revisions:
//   - 2026-11-01 23:00: initial creation, before the encoder and decoder factories
//   - 2026-11-02 16:45: extra padding is refused, not ignored (C22)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_KeepsTheNameAndTheTextOfEachBuiltin(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
		wantErr    string
	}{
		{
			"encode pads",
			`base64.encode(b"hi\xff~")`,
			`"aGn/fg=="`,
			"",
		},
		{
			"encode a string",
			`base64.encode("x")`,
			`"eA=="`,
			"",
		},
		{
			"encode no argument",
			`base64.encode()`,
			"",
			"base64.encode: got 0 arguments, want 1",
		},
		{
			"encode two arguments",
			`base64.encode(b"a", b"b")`,
			"",
			"base64.encode: got 2 arguments, want 1",
		},
		{
			"encode a number",
			`base64.encode(1)`,
			"",
			"base64.encode got int: wants bytes or a string",
		},
		{
			"encode a keyword",
			`base64.encode(data=b"a")`,
			"",
			"base64.encode: unexpected keyword arguments",
		},
		{
			"urlencode does not pad",
			`base64.urlencode(b"hi\xff~")`,
			`"aGn_fg"`,
			"",
		},
		{
			"urlencode a string",
			`base64.urlencode("x")`,
			`"eA"`,
			"",
		},
		{
			"urlencode no argument",
			`base64.urlencode()`,
			"",
			"base64.urlencode: got 0 arguments, want 1",
		},
		{
			"urlencode two arguments",
			`base64.urlencode(b"a", b"b")`,
			"",
			"base64.urlencode: got 2 arguments, want 1",
		},
		{
			"urlencode a number",
			`base64.urlencode(1)`,
			"",
			"base64.urlencode got int: wants bytes or a string",
		},
		{
			"urlencode a keyword",
			`base64.urlencode(data=b"a")`,
			"",
			"base64.urlencode: unexpected keyword arguments",
		},
		{
			"decode padded",
			`base64.decode("aGk=")`,
			`b"hi"`,
			"",
		},
		{
			"decode unpadded",
			`base64.decode("aGk")`,
			`b"hi"`,
			"",
		},
		{
			"decode extra padding",
			`base64.decode("aGk==")`,
			"",
			`base64.decode got "aGk==": not the encoding this reads`,
		},
		{
			"decode standard characters",
			`base64.decode("+/+/")`,
			`b"\xfb\xff\xbf"`,
			"",
		},
		{
			"decode no argument",
			`base64.decode()`,
			"",
			"base64.decode: got 0 arguments, want 1",
		},
		{
			"decode two arguments",
			`base64.decode("a", "b")`,
			"",
			"base64.decode: got 2 arguments, want 1",
		},
		{
			"decode a number",
			`base64.decode(1)`,
			"",
			"base64.decode: for parameter 1: got int, want string",
		},
		{
			"decode bytes",
			`base64.decode(b"aGk=")`,
			"",
			"base64.decode: for parameter 1: got bytes, want string",
		},
		{
			"decode a keyword",
			`base64.decode(text="aGk")`,
			"",
			"base64.decode: unexpected keyword arguments",
		},
		{
			"decode not base64",
			`base64.decode("!!!!")`,
			"",
			`base64.decode got "!!!!": not the encoding this reads`,
		},
		{
			"decode url characters",
			`base64.decode("-_-_")`,
			"",
			`base64.decode got "-_-_": not the encoding this reads`,
		},
		{
			"decode one character",
			`base64.decode("a")`,
			"",
			`base64.decode got "a": not the encoding this reads`,
		},
		{
			"decode non-ASCII",
			`base64.decode("é")`,
			"",
			`base64.decode got "é": not the encoding this reads`,
		},
		{
			"urldecode padded",
			`base64.urldecode("aGk=")`,
			`b"hi"`,
			"",
		},
		{
			"urldecode unpadded",
			`base64.urldecode("aGk")`,
			`b"hi"`,
			"",
		},
		{
			"urldecode extra padding",
			`base64.urldecode("aGk==")`,
			"",
			`base64.urldecode got "aGk==": not the encoding this reads`,
		},
		{
			"urldecode url characters",
			`base64.urldecode("-_-_")`,
			`b"\xfb\xff\xbf"`,
			"",
		},
		{
			"urldecode no argument",
			`base64.urldecode()`,
			"",
			"base64.urldecode: got 0 arguments, want 1",
		},
		{
			"urldecode two arguments",
			`base64.urldecode("a", "b")`,
			"",
			"base64.urldecode: got 2 arguments, want 1",
		},
		{
			"urldecode a number",
			`base64.urldecode(1)`,
			"",
			"base64.urldecode: for parameter 1: got int, want string",
		},
		{
			"urldecode bytes",
			`base64.urldecode(b"aGk=")`,
			"",
			"base64.urldecode: for parameter 1: got bytes, want string",
		},
		{
			"urldecode a keyword",
			`base64.urldecode(text="aGk")`,
			"",
			"base64.urldecode: unexpected keyword arguments",
		},
		{
			"urldecode not base64",
			`base64.urldecode("!!!!")`,
			"",
			`base64.urldecode got "!!!!": not the encoding this reads`,
		},
		{
			"urldecode standard characters",
			`base64.urldecode("+/+/")`,
			"",
			`base64.urldecode got "+/+/": not the encoding this reads`,
		},
		{
			"urldecode one character",
			`base64.urldecode("a")`,
			"",
			`base64.urldecode got "a": not the encoding this reads`,
		},
		{
			"urldecode non-ASCII",
			`base64.urldecode("é")`,
			"",
			`base64.urldecode got "é": not the encoding this reads`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)

				switch {
				case item.wantErr == "" && err != nil:
					t.Fatalf(
						"%s failed: %v",
						item.expression,
						err,
					)
				case item.wantErr != "" && err == nil:
					t.Fatalf(
						"%s gave %s, want error %q",
						item.expression,
						got,
						item.wantErr,
					)
				case item.wantErr != "" && err.Error() != item.wantErr:
					t.Fatalf(
						"%s gave error %q, want %q",
						item.expression,
						err.Error(),
						item.wantErr,
					)
				case got != item.want:
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						item.want,
					)
				}
			},
		)
	}
}
