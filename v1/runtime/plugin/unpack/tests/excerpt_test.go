package tests

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestQuotePrefix_Table feeds QuotePrefix the shapes the Q4 decision names and compares
// the whole result.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQuotePrefix_Table(t *testing.T) {
	var (
		cut   = common.QUOTE_LIMIT
		exact = strings.Repeat(
			"a",
			cut,
		)
		over = strings.Repeat(
			"a",
			cut+1,
		)
		prefix = strings.Repeat(
			"a",
			cut,
		)
		five = strings.Repeat(
			"a",
			5_000_000,
		)
		split2 = strings.Repeat(
			"a",
			cut-1,
		) + "éz"
		split3 = strings.Repeat(
			"a",
			cut-1,
		) + "€z"
		split4 = strings.Repeat(
			"a",
			cut-1,
		) + "\U0001F600z"
		endrune = strings.Repeat(
			"a",
			cut-2,
		) + "éz"
		stray = strings.Repeat(
			"\x80",
			cut+6,
		)
		inject = strings.Repeat(
			"a",
			cut-3,
		) + "\nFAKE LOG LINE\n"
	)

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			"empty",
			"",
			`""`,
		},
		{
			"short",
			"abc",
			`"abc"`,
		},
		{
			"exactly N",
			exact,
			strconv.Quote(exact),
		},
		{
			"N plus one",
			over,
			strconv.Quote(prefix) + "... (101 bytes)",
		},
		{
			"5 MB",
			five,
			strconv.Quote(prefix) + "... (5000000 bytes)",
		},
		{
			"2-byte rune split by the cut",
			split2,
			strconv.Quote(strings.Repeat(
				"a",
				cut-1,
			)) + "... (" + strconv.Itoa(len(split2)) + " bytes)",
		},
		{
			"3-byte rune split by the cut",
			split3,
			strconv.Quote(strings.Repeat(
				"a",
				cut-1,
			)) + "... (" + strconv.Itoa(len(split3)) + " bytes)",
		},
		{
			"4-byte rune split by the cut",
			split4,
			strconv.Quote(strings.Repeat(
				"a",
				cut-1,
			)) + "... (" + strconv.Itoa(len(split4)) + " bytes)",
		},
		{
			"2-byte rune ends at the cut",
			endrune,
			strconv.Quote(strings.Repeat(
				"a",
				cut-2,
			)+"é") + "... (" + strconv.Itoa(len(endrune)) + " bytes)",
		},
		{
			"invalid UTF-8 keeps N bytes",
			stray,
			_Marked(
				stray[:cut],
				stray,
			),
		},
		{
			"newline injection stays escaped",
			inject,
			_Marked(
				inject[:cut],
				inject,
			),
		},
		{
			"newline, short",
			"a\nb",
			`"a\nb"`,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := unpack.QuotePrefix(test.text)
				if got != test.want {
					t.Fatalf(
						"QuotePrefix = %.200q, want %.200q",
						got,
						test.want,
					)
				}

				if strings.ContainsAny(
					got,
					"\n\r",
				) {
					t.Fatalf(
						"QuotePrefix holds a raw line break: %.200q",
						got,
					)
				}

				if !utf8.ValidString(got) {
					t.Fatalf(
						"QuotePrefix is not valid UTF-8: %.200q",
						got,
					)
				}
			},
		)
	}
}

// TestQuotePrefix_ShortTextEqualsPercentQ checks that text of N bytes or fewer gets
// the bytes that %q writes.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQuotePrefix_ShortTextEqualsPercentQ(t *testing.T) {
	for _, text := range []string{
		"",
		"x",
		"\t",
		"\xff\xfe",
		"héllo",
		"a\"b\\c",
		"\u202eab",
		"a\nFAKE",
	} {
		t.Run(
			strconv.Quote(text),
			func(t *testing.T) {
				want := fmt.Sprintf(
					"%q",
					text,
				)
				if got := unpack.QuotePrefix(text); got != want {
					t.Fatalf(
						"QuotePrefix = %q, want %q",
						got,
						want,
					)
				}
			},
		)
	}
}

// TestQuotePrefix_BoundsHostileInput checks the size bound 4N+2 plus the marker on
// 5 MB inputs of every escape width.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQuotePrefix_BoundsHostileInput(t *testing.T) {
	const SIZE = 5_000_000

	for name, unit := range map[string]string{
		"ascii":   "a",
		"0xff":    "\xff",
		"newline": "\n",
		"4-byte":  "\U0001F600",
		"0x80":    "\x80",
		"bidi":    "\u202e",
	} {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					text = strings.Repeat(
						unit,
						SIZE/len(unit),
					)
					got   = unpack.QuotePrefix(text)
					limit = 4*common.QUOTE_LIMIT + 2 + len("... ( bytes)") + 7
				)

				if len(got) > limit {
					t.Fatalf(
						"QuotePrefix is %d bytes, want at most %d",
						len(got),
						limit,
					)
				}

				if !strings.HasSuffix(
					got,
					" bytes)",
				) {
					t.Fatalf(
						"QuotePrefix lacks the marker: %.200q",
						got,
					)
				}
			},
		)
	}
}

// TestQuotePrefix_NeverSplitsARune cuts a string of 3-byte runes at every offset
// and expects no \x escape in the result.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQuotePrefix_NeverSplitsARune(t *testing.T) {
	for pad := range 4 {
		var (
			text = strings.Repeat(
				"a",
				pad,
			) + strings.Repeat(
				"€",
				80,
			)
			got = unpack.QuotePrefix(text)
		)

		if strings.Contains(
			got,
			`\x`,
		) {
			t.Fatalf(
				"pad %d: QuotePrefix split a rune: %.200q",
				pad,
				got,
			)
		}
	}
}

// TestShortText_Table is the table of ShortText, the unquoted form for text that the
// code prints as it is today.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestShortText_Table(t *testing.T) {
	var (
		exact = strings.Repeat(
			"7",
			common.QUOTE_LIMIT,
		)
		over = strings.Repeat(
			"7",
			common.QUOTE_LIMIT+1,
		)
		wide = strings.Repeat(
			"a",
			common.QUOTE_LIMIT-1,
		) + "€"
	)

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			"empty",
			"",
			"",
		},
		{
			"short",
			"-12",
			"-12",
		},
		{
			"exactly N",
			exact,
			exact,
		},
		{
			"N plus one",
			over,
			exact + "... (101 bytes)",
		},
		{
			"rune split by the cut",
			wide,
			strings.Repeat(
				"a",
				common.QUOTE_LIMIT-1,
			) + "... (" + strconv.Itoa(len(wide)) + " bytes)",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				if got := unpack.ShortText(test.text); got != test.want {
					t.Fatalf(
						"ShortText = %q, want %q",
						got,
						test.want,
					)
				}
			},
		)
	}
}

// FuzzQuotePrefix checks, for any text, the three promises of QuotePrefix: valid UTF-8,
// no raw line break, and the size bound.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzQuotePrefix(f *testing.F) {
	f.Add("")
	f.Add("a\nb")
	f.Add(strings.Repeat(
		"\xe2\x82",
		80,
	))

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		got := unpack.QuotePrefix(text)
		if !utf8.ValidString(got) || strings.ContainsAny(
			got,
			"\n\r",
		) {
			t.Fatalf(
				"QuotePrefix(%q) = %q",
				text,
				got,
			)
		}

		if len(text) <= common.QUOTE_LIMIT {
			if got != strconv.Quote(text) {
				t.Fatalf(
					"QuotePrefix(%q) = %q, want the %q text",
					text,
					got,
					strconv.Quote(text),
				)
			}

			return
		}

		if len(got) > 4*common.QUOTE_LIMIT+2+len("... ( bytes)")+20 {
			t.Fatalf(
				"QuotePrefix is %d bytes for %d",
				len(got),
				len(text),
			)
		}
	})
}

// TestShortPath_Table checks the file name in an operating system error: a long
// name is cut, a short name and an error of another type stay as they are,
// and errors.Is still finds the cause.
//
// Revisions:
//   - 2026-11-02 11:03: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestShortPath_Table(t *testing.T) {
	var (
		long = "/" + strings.Repeat(
			"z",
			1_000_000,
		)
		other = errors.New("other")
		short = &fs.PathError{
			Op:   "open",
			Path: "/x",
			Err:  fs.ErrNotExist,
		}
		held = &fs.PathError{
			Op:   "open",
			Path: long,
			Err:  fs.ErrNotExist,
		}
		linked = &os.LinkError{
			Op:  "rename",
			Old: long,
			New: "/y",
			Err: fs.ErrExist,
		}
	)

	tests := []struct {
		name string
		in   error
		want string
		is   error
	}{
		{
			"short path",
			short,
			short.Error(),
			fs.ErrNotExist,
		},
		{
			"long path",
			held,
			"open " + unpack.ShortText(long) + ": file does not exist",
			fs.ErrNotExist,
		},
		{
			"long old name",
			linked,
			"rename " + unpack.ShortText(long) + " /y: file already exists",
			fs.ErrExist,
		},
		{
			"other error",
			other,
			"other",
			other,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				got := unpack.ShortPath(test.in)
				if got.Error() != test.want {
					t.Fatalf(
						"Path = %q, want %q",
						got.Error(),
						test.want,
					)
				}

				if !errors.Is(
					got,
					test.is,
				) {
					t.Fatalf(
						"Path lost %v",
						test.is,
					)
				}
			},
		)
	}
}

// _Marked is the text QuotePrefix must give for whole when it keeps prefix: the
// quoted prefix, the dots outside the quote, and the length of whole.
//
// Revisions:
//   - 2026-11-02 11:06: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Marked(
	prefix string,
	whole string,
) string {
	return strconv.Quote(prefix) + "... (" + strconv.Itoa(len(whole)) + " bytes)"
}
