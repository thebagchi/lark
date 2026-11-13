package tests

// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: RFC 4648's own vectors. A
// table written from the implementation would agree with it by construction.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	stdbase64 "encoding/base64"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// SCRIPT is what a failure calls the expression it was given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	SCRIPT = "base64_test.star"
)

// ENCODED is what this package's own sentinel is reached by, without naming
// the package under test twice in every row.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	ENCODED = _Sentinel()
)

// _Sentinel is unpack.ErrEncoded, the value both decoders raise.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-08 13:25: returns unpack.ErrEncoded, base64's being merged (C98, Q251)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Sentinel() error {
	return unpack.ErrEncoded
}

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-09-23 23:20: moved here with the encodings it exercises
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Eval(
	t *testing.T,
	expression string,
) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		expression,
		env,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Check runs a table of expression / expected pairs.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-09-23 23:20: moved here
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Check(
	t *testing.T,
	cases []struct{ name, expression, want string },
) {
	t.Helper()

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.expression,
						err,
					)
				}

				if got != item.want {
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

// TestEncode_MatchesRFC4648 is the whole of the standard alphabet's vectors.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's TestBase64_MatchesRFC4648
//   - 2026-09-23 23:20: spells the calls as members of the base64 module
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestEncode_MatchesRFC4648(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"empty",
				`base64.encode("")`,
				`""`,
			},
			{
				"one byte",
				`base64.encode("f")`,
				`"Zg=="`,
			},
			{
				"two",
				`base64.encode("fo")`,
				`"Zm8="`,
			},
			{
				"three",
				`base64.encode("foo")`,
				`"Zm9v"`,
			},
			{
				"four",
				`base64.encode("foob")`,
				`"Zm9vYg=="`,
			},
			{
				"five",
				`base64.encode("fooba")`,
				`"Zm9vYmE="`,
			},
			{
				"six",
				`base64.encode("foobar")`,
				`"Zm9vYmFy"`,
			},
			{
				"of bytes",
				`base64.encode(b"\x00\xff")`,
				`"AP8="`,
			},
			{
				"decode padded",
				`base64.decode("Zm9vYmE=")`,
				`b"fooba"`,
			},
			{
				"decode unpadded",
				`base64.decode("Zm9vYmE")`,
				`b"fooba"`,
			},
			{
				"decode empty",
				`base64.decode("")`,
				`b""`,
			},
			{
				"round trip",
				`base64.decode(base64.encode(b"\x00\xff"))`,
				`b"\x00\xff"`,
			},
		},
	)
}

// TestURLEncode_UsesTheOtherTwoCharacters is section 5, which differs from
// section 4 in exactly two characters and in carrying no padding.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's
//     TestBase64URL_UsesTheOtherTwoCharacters
//   - 2026-09-23 23:20: spells the calls as members of the base64 module
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestURLEncode_UsesTheOtherTwoCharacters(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"standard",
				`base64.encode(b"\xfb\xff\xbf")`,
				`"+/+/"`,
			},
			{
				"url safe",
				`base64.urlencode(b"\xfb\xff\xbf")`,
				`"-_-_"`,
			},
			{
				"url is unpadded",
				`base64.urlencode("f")`,
				`"Zg"`,
			},
			{
				"url decode unpadded",
				`base64.urldecode("Zg")`,
				`b"f"`,
			},
			{
				"url decode padded",
				`base64.urldecode("Zg==")`,
				`b"f"`,
			},
			{
				"round trip",
				`base64.urldecode(base64.urlencode(b"\xfb\xff\xbf"))`,
				`b"\xfb\xff\xbf"`,
			},
		},
	)
}

// TestBase64_RefusesWhatItCannotRead records that each alphabet reads its own
// and not the other's.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, in codec
//   - 2026-09-23 23:20: moved here
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_RefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{
			"of a number",
			`base64.encode(1)`,
			unpack.ErrData.Error(),
		},
		{
			"not base64",
			`base64.decode("!!!!")`,
			ENCODED.Error(),
		},
		{
			"the url alphabet is not the standard one",
			`base64.decode("-_-_")`,
			ENCODED.Error(),
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s succeeded, want %s",
						item.expression,
						item.want,
					)
				}

				if !_Names(
					err,
					item.want,
				) {
					t.Fatalf(
						"%s gave %v, want %s",
						item.expression,
						err,
						item.want,
					)
				}
			},
		)
	}
}

// _Names reports whether an error says what it should.
//
// The text rather than the sentinel, because an evaluation error carries the
// script position and wraps what the builtin returned.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Names(
	err error,
	want string,
) bool {
	return err != nil && strings.Contains(
		err.Error(),
		want,
	)
}

// _Refuses runs a table of expressions that must raise unpack.ErrEncoded.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Refuses(
	t *testing.T,
	cases []struct{ name, expression string },
) {
	t.Helper()

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						ENCODED,
					)
				}

				if !_Names(
					err,
					ENCODED.Error(),
				) {
					t.Fatalf(
						"%s gave %v, want %s",
						item.expression,
						err,
						ENCODED,
					)
				}
			},
		)
	}
}

// TestBase64_KeepsTheCanonicalSpellings pins what stays accepted after C22:
// no padding, and exactly the padding RFC 4648 section 4 writes. Each row
// passed before the change, so a failure here is a regression.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 sections 3.2 and 4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_KeepsTheCanonicalSpellings(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"one byte padded",
				`base64.decode("Zg==")`,
				`b"f"`,
			},
			{
				"one byte unpadded",
				`base64.decode("Zg")`,
				`b"f"`,
			},
			{
				"two bytes padded",
				`base64.decode("Zm8=")`,
				`b"fo"`,
			},
			{
				"two bytes unpadded",
				`base64.decode("Zm8")`,
				`b"fo"`,
			},
			{
				"whole quantum",
				`base64.decode("Zm9v")`,
				`b"foo"`,
			},
			{
				"four bytes padded",
				`base64.decode("Zm9vYg==")`,
				`b"foob"`,
			},
			{
				"five bytes unpadded",
				`base64.decode("Zm9vYmE")`,
				`b"fooba"`,
			},
			{
				"url one byte padded",
				`base64.urldecode("Zg==")`,
				`b"f"`,
			},
			{
				"url one byte unpadded",
				`base64.urldecode("Zg")`,
				`b"f"`,
			},
			{
				"url two bytes padded",
				`base64.urldecode("Zm8=")`,
				`b"fo"`,
			},
			{
				"url alphabet",
				`base64.urldecode("-_-_")`,
				`b"\xfb\xff\xbf"`,
			},
		},
	)
}

// TestBase64_RefusesExcessOrWrongPadding is RFC 4648 section 3.3 and section 4:
// a decoder rejects what its referring specification does not allow, and lark
// states no rule that ignores excess "=".
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 section 3.3)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_RefusesExcessOrWrongPadding(t *testing.T) {
	_Refuses(
		t,
		[]struct{ name, expression string }{
			{
				"one too few",
				`base64.decode("Zg=")`,
			},
			{
				"one too many",
				`base64.decode("Zg===")`,
			},
			{
				"eight",
				`base64.decode("Zg========")`,
			},
			{
				"only padding",
				`base64.decode("=")`,
			},
			{
				"padding on a whole quantum",
				`base64.decode("Zm9v=")`,
			},
			{
				"two on a whole quantum",
				`base64.decode("Zm9v==")`,
			},
			{
				"two on three characters",
				`base64.decode("Zm8==")`,
			},
			{
				"url one too few",
				`base64.urldecode("Zg=")`,
			},
			{
				"url one too many",
				`base64.urldecode("Zg===")`,
			},
			{
				"padding before the end",
				`base64.decode("Zg==Zg==")`,
			},
			{
				"padding inside",
				`base64.decode("Z=g")`,
			},
			{
				"space",
				`base64.decode("Zm 9v")`,
			},
			{
				"tab",
				`base64.decode("Zm\t9v")`,
			},
			{
				"nul",
				`base64.decode("Zg\x00")`,
			},
		},
	)
}

// TestBase64_RefusesNonZeroPadBits is RFC 4648 section 3.5: a decoder MAY
// reject pad bits that are not zero, and lark does, as there is otherwise
// more than one text for one value.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 section 3.5)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_RefusesNonZeroPadBits(t *testing.T) {
	_Refuses(
		t,
		[]struct{ name, expression string }{
			{
				"padded",
				`base64.decode("Zh==")`,
			},
			{
				"unpadded",
				`base64.decode("Zh")`,
			},
			{
				"four low bits",
				`base64.decode("Zm9vYh")`,
			},
			{
				"two low bits",
				`base64.decode("Zm9vYmF")`,
			},
			{
				"two low bits padded",
				`base64.decode("Zm9=")`,
			},
			{
				"url unpadded",
				`base64.urldecode("Zh")`,
			},
			{
				"url padded",
				`base64.urldecode("Zh==")`,
			},
		},
	)
}

// TestBase64_RefusesLineBreaks is RFC 4648 section 3.3 (a line break is a
// non-alphabet character) and RFC 7515 section 2 (base64url has no line
// breaks). Go skips CR and LF even in Strict mode, so lark scans for them.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 section 3.3)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase64_RefusesLineBreaks(t *testing.T) {
	_Refuses(
		t,
		[]struct{ name, expression string }{
			{
				"lf inside",
				`base64.decode("Zm9v\nYmFy")`,
			},
			{
				"crlf inside",
				`base64.decode("Zm9v\r\nYmFy")`,
			},
			{
				"cr inside",
				`base64.decode("Zm9v\rYmFy")`,
			},
			{
				"lf leading",
				`base64.decode("\nZm9v")`,
			},
			{
				"lf trailing",
				`base64.decode("Zm9v\n")`,
			},
			{
				"lf before padding",
				`base64.decode("Zg\n==")`,
			},
			{
				"lf after padding",
				`base64.decode("Zg==\n")`,
			},
			{
				"url lf inside",
				`base64.urldecode("Zm9v\nYmFy")`,
			},
			{
				"url crlf trailing",
				`base64.urldecode("Zg\r\n")`,
			},
			{
				"url lf leading",
				`base64.urldecode("\nZg")`,
			},
		},
	)
}

// FuzzDecode feeds both base64 decoders any text and asserts the RFC 4648 rules
// on the result. Accepted text carries no CR or LF (s3.3) and re-encodes to the
// text without its padding, so one value has one spelling (s3.5, s12). Refused
// text raises unpack.ErrEncoded and never panics. The seeds are the T1-T18 cases of
// the Q8 research.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"", "Zg==", "Zg", "Zm8=", "Zm9v", "Zm9vYmE", "Zg=", "Zg===", "Zg========",
		"=", "Zm9v=", "Zm9v==", "Zm8==", "Zh==", "Zh", "Zm9vYh", "Zm9vYmF",
		"Zm9v\r\nYmFy", "Zg\n==", "Zg=\n", "\nZg", "Zg=a", "Z=g", "Zg==Zg==",
		"Zm 9v", "Zm\t9v", "Zg\x00", "-_-_", "+/+/",
	} {
		f.Add(seed)
	}

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		f.Fatalf(
			"environment: %v",
			err,
		)
	}

	module := env["base64"].(starlark.HasAttrs)

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		for _, item := range []struct {
			name     string
			encoding *stdbase64.Encoding
		}{
			{
				"decode",
				stdbase64.RawStdEncoding,
			},
			{
				"urldecode",
				stdbase64.RawURLEncoding,
			},
		} {
			var builtin, _ = module.Attr(item.name)

			value, err := starlark.Call(
				&starlark.Thread{
					Name: SCRIPT,
				},
				builtin,
				starlark.Tuple{starlark.String(text)},
				nil,
			)
			if err != nil {
				if !_Names(
					err,
					ENCODED.Error(),
				) {
					t.Fatalf(
						"%s(%q) gave %v, want %s",
						item.name,
						text,
						err,
						ENCODED,
					)
				}

				continue
			}

			if strings.ContainsAny(
				text,
				"\r\n",
			) {
				t.Fatalf(
					"%s(%q) accepted a line break",
					item.name,
					text,
				)
			}

			got := item.encoding.EncodeToString([]byte(value.(starlark.Bytes)))
			if want := strings.TrimRight(
				text,
				"=",
			); got != want {
				t.Fatalf(
					"%s(%q) re-encodes to %q, want %q",
					item.name,
					text,
					got,
					want,
				)
			}
		}
	})
}
