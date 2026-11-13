package tests

// These tests are table driven because the thing under test is a
// specification, and the rows are RFC 4648's own vectors. A table written from
// the implementation would agree with it by construction.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	stdbase32 "encoding/base32"
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// SCRIPT is what a failure calls the expression it was given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	SCRIPT = "base32_test.star"
)

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
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

// TestEncode_MatchesRFC4648 is that document's base32 vectors.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, in codec
//   - 2026-09-24 00:53: moved here, spelled as members of the base32 module
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestEncode_MatchesRFC4648(t *testing.T) {
	cases := []struct{ name, expression, want string }{
		{
			"empty",
			`base32.encode("")`,
			`""`,
		},
		{
			"one",
			`base32.encode("f")`,
			`"MY======"`,
		},
		{
			"two",
			`base32.encode("fo")`,
			`"MZXQ===="`,
		},
		{
			"three",
			`base32.encode("foo")`,
			`"MZXW6==="`,
		},
		{
			"four",
			`base32.encode("foob")`,
			`"MZXW6YQ="`,
		},
		{
			"five",
			`base32.encode("fooba")`,
			`"MZXW6YTB"`,
		},
		{
			"six",
			`base32.encode("foobar")`,
			`"MZXW6YTBOI======"`,
		},
		{
			"decode padded",
			`base32.decode("MZXW6YTBOI======")`,
			`b"foobar"`,
		},
		{
			"decode unpadded",
			`base32.decode("MZXW6YTBOI")`,
			`b"foobar"`,
		},
		{
			"round trip",
			`base32.decode(base32.encode(b"\x00\xff"))`,
			`b"\x00\xff"`,
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

// TestBase32_RefusesWhatItCannotRead records the two mistakes a caller can
// make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase32_RefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       error
	}{
		{
			"of a number",
			`base32.encode(1)`,
			unpack.ErrData,
		},
		{
			"not base32",
			`base32.decode("1111")`,
			unpack.ErrEncoded,
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
				if !errors.Is(
					err,
					item.want,
				) {
					t.Fatalf(
						"%s gave %v, want %v",
						item.expression,
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestBase32_KeepsTheCanonicalSpellings pins what stays accepted after C22:
// no padding, and exactly the padding RFC 4648 section 6 writes. Each row
// passed before the change.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 sections 3.2 and 6)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase32_KeepsTheCanonicalSpellings(t *testing.T) {
	cases := []struct{ name, expression, want string }{
		{
			"empty",
			`base32.decode("")`,
			`b""`,
		},
		{
			"one padded",
			`base32.decode("MY======")`,
			`b"f"`,
		},
		{
			"one unpadded",
			`base32.decode("MY")`,
			`b"f"`,
		},
		{
			"two padded",
			`base32.decode("MZXQ====")`,
			`b"fo"`,
		},
		{
			"two unpadded",
			`base32.decode("MZXQ")`,
			`b"fo"`,
		},
		{
			"three padded",
			`base32.decode("MZXW6===")`,
			`b"foo"`,
		},
		{
			"three unpadded",
			`base32.decode("MZXW6")`,
			`b"foo"`,
		},
		{
			"four padded",
			`base32.decode("MZXW6YQ=")`,
			`b"foob"`,
		},
		{
			"four unpadded",
			`base32.decode("MZXW6YQ")`,
			`b"foob"`,
		},
		{
			"five",
			`base32.decode("MZXW6YTB")`,
			`b"fooba"`,
		},
		{
			"six padded",
			`base32.decode("MZXW6YTBOI======")`,
			`b"foobar"`,
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

// TestBase32_RefusesWhatRFC4648Forbids is RFC 4648 section 3.3 (padding and
// non-alphabet characters), section 3.5 (pad bits) and section 6 (the final
// group has 2, 4, 5, 7 or 8 characters; 1, 3 and 6 carry no whole byte, and
// were dropped without an error before C22).
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 4648 sections 3.3, 3.5 and 6)
//   - 2026-11-02 16:45: rows with zero pad bits in the final group, so only the
//     group check refuses them (C22)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestBase32_RefusesWhatRFC4648Forbids(t *testing.T) {
	cases := []struct{ name, expression string }{
		{
			"one too many padding",
			`base32.decode("MZXW6YTBOI=========")`,
		},
		{
			"wrong padding count",
			`base32.decode("MZXW6YTBOI=")`,
		},
		{
			"only padding",
			`base32.decode("========")`,
		},
		{
			"padding on a whole group",
			`base32.decode("MZXW6YTB=")`,
		},
		{
			"pad bits padded",
			`base32.decode("MZXW6YTBOJ======")`,
		},
		{
			"pad bits unpadded",
			`base32.decode("MZXW6YTBOJ")`,
		},
		{
			"pad bits of two",
			`base32.decode("MZ======")`,
		},
		{
			"pad bits of four",
			`base32.decode("MZXR====")`,
		},
		{
			"pad bits of five",
			`base32.decode("MZXW7===")`,
		},
		{
			"pad bits of seven",
			`base32.decode("MZXW6YR=")`,
		},
		{
			"lf inside",
			`base32.decode("MY\nMY")`,
		},
		{
			"crlf trailing",
			`base32.decode("MZXW6YTB\r\n")`,
		},
		{
			"lf leading",
			`base32.decode("\nMZXW6YTB")`,
		},
		{
			"space",
			`base32.decode("MZXW 6YTB")`,
		},
		{
			"lower case",
			`base32.decode("my======")`,
		},
		{
			"final group of one",
			`base32.decode("M")`,
		},
		{
			"final group of three",
			`base32.decode("MZX")`,
		},
		{
			"final group of six",
			`base32.decode("MZXW6Y")`,
		},
		{
			"nine characters",
			`base32.decode("MZXW6YTBO")`,
		},
		{
			"eleven characters",
			`base32.decode("MZXW6YTBOIM")`,
		},
		{
			"padded group of three",
			`base32.decode("MZX=====")`,
		},
		{
			"one with no pad bits set",
			`base32.decode("A")`,
		},
		{
			"three with no pad bits set",
			`base32.decode("AAA")`,
		},
		{
			"six with no pad bits set",
			`base32.decode("AAAAAA")`,
		},
		{
			"nine with no pad bits set",
			`base32.decode("MZXW6YAAA")`,
		},
		{
			"padded one with no pad bits set",
			`base32.decode("A=======")`,
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
				if err == nil {
					t.Fatalf(
						"%s gave %s, want %v",
						item.expression,
						got,
						unpack.ErrEncoded,
					)
				}

				if !strings.Contains(
					err.Error(),
					unpack.ErrEncoded.Error(),
				) {
					t.Fatalf(
						"%s gave %v, want %v",
						item.expression,
						err,
						unpack.ErrEncoded,
					)
				}
			},
		)
	}
}

// FuzzDecode feeds base32.decode any text and asserts the RFC 4648 rules on the
// result. Accepted text carries no CR or LF (s3.3) and re-encodes to the text
// without its padding (s3.5, s6). Refused text raises unpack.ErrEncoded and never
// panics. The seeds are the T1-T18 cases of the Q8 research.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-09 11:16: the corpus file becomes an f.Add call (C114, Q283)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"", "MY======", "MY", "MZXQ====", "MZXW6===", "MZXW6YQ=", "MZXW6YTB",
		"MZXW6YTBOI======", "MZXW6YTBOI", "MZXW6YTBOI=========", "MZXW6YTBOI=",
		"MZXW6YTBOJ======", "MZ======", "MZXW6YTBOJ", "MY\nMY", "my======",
		"M", "MZX", "MZXW6Y", "MZXW6YTBO", "MZXW6YTBOIM", "MZX=====", "========",
		"A", "AAA", "AAAAAA", "MZXW6YAAA", "A=======",
	} {
		f.Add(seed)
	}

	// A finding of the fuzzer, once a corpus file: text with bytes above 0x7f.
	f.Add("22222\xff\xff\xff")

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		f.Fatalf(
			"environment: %v",
			err,
		)
	}

	var (
		module     = env["base32"].(starlark.HasAttrs)
		builtin, _ = module.Attr("decode")
	)

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		value, err := starlark.Call(
			&starlark.Thread{
				Name: SCRIPT,
			},
			builtin,
			starlark.Tuple{starlark.String(text)},
			nil,
		)
		if err != nil {
			if !strings.Contains(
				err.Error(),
				unpack.ErrEncoded.Error(),
			) {
				t.Fatalf(
					"decode(%q) gave %v, want %v",
					text,
					err,
					unpack.ErrEncoded,
				)
			}

			return
		}

		if strings.ContainsAny(
			text,
			"\r\n",
		) {
			t.Fatalf(
				"decode(%q) accepted a line break",
				text,
			)
		}

		var (
			bare = stdbase32.StdEncoding.WithPadding(stdbase32.NoPadding)
			got  = bare.EncodeToString([]byte(value.(starlark.Bytes)))
		)

		if want := strings.TrimRight(
			text,
			"=",
		); got != want {
			t.Fatalf(
				"decode(%q) re-encodes to %q, want %q",
				text,
				got,
				want,
			)
		}
	})
}
