// An error that quotes what a script supplied quotes a short prefix of it.
//
// Revisions:
//   - 2026-11-02 11:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// PLACE is the position a run-time error carries, in the file these sites run.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	PLACE = regexp.MustCompile(`probe\.star:\d+: `)
)

// _Site is one builtin that puts script text into an error.
//
// A script binds x to short or to long, then runs body, which must fail. The
// text of the failure with short is pinned to what the code wrote before
// change C21. The text with long must stay within the size bound.
//
// A site that a script reaches only at the top level sets top instead of body:
// top is one line of the module with X where the input goes.
//
// Revisions:
//   - 2026-11-02 11:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Site struct {
	name   string
	top    string
	body   string
	short  string
	want   string
	long   string
	quotes int
}

// _RunSite compiles and runs a script that binds x to expr and runs body, with
// the file plugin added. It returns the error text.
//
// Revisions:
//   - 2026-11-02 11:00: initial creation
//   - 2026-11-04 06:39: drops the position a run-time error now begins with (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-05 17:15: main reads the binding of a top-level site, which the
//     shake would drop otherwise (C44b, Q163)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _RunSite(
	t *testing.T,
	site *_Site,
	expr string,
) string {
	t.Helper()

	text := "def main():\n    x = " + expr + "\n    " + site.body + "\n"
	if site.top != "" {
		// A binding nothing reads is shaken out of the compile (Q163), so
		// main reads the one a site binds.
		use := "pass"
		if strings.HasPrefix(
			site.top,
			"V = ",
		) {
			use = "print(V)"
		}

		text = strings.Replace(
			site.top,
			"X",
			expr,
			1,
		) + "\ndef main():\n    " + use + "\n"
	}

	built, err := runtime.Compile(
		&common.Source{
			Entry: "probe.star",
			Text:  []byte(text),
		},
		runtime.WithPlugins(file.New()),
	)
	if err != nil {
		return err.Error()
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()
	if err == nil {
		t.Fatalf(
			"site %q gave no error",
			site.name,
		)
	}

	// The position of the call is not what these tests pin: the text of the
	// builtin is.
	return PLACE.ReplaceAllString(
		err.Error(),
		"",
	)
}

// _Sites is every builtin of the inventory that a script can reach with a
// bad input.
//
// Revisions:
//   - 2026-11-02 11:00: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sites() []_Site {
	const (
		BIG    = `"z" * 5000000`
		NUMBER = `int("1" + "0" * 500)`
	)

	var huge = `"` + strings.Repeat(
		"z",
		5_000_000,
	) + `"`

	return []_Site{
		{
			name:  "hex.to_bytes",
			body:  "hex.to_bytes(x)",
			want:  "hex.to_bytes got \"zz\": not hexadecimal",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name:  "hex.to_binary",
			body:  "hex.to_binary(x)",
			want:  "hex.to_binary got \"zz\": not hexadecimal",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name:  "hex.to_int of only a prefix",
			body:  "hex.to_int(x)",
			want:  "hex.to_int got \"0x\": not hexadecimal",
			short: `"0x"`,
		},
		{
			name:  "binary.to_int",
			body:  "binary.to_int(x)",
			want:  "binary.to_int got \"2\": not a bit string",
			short: `"2"`,
			long:  BIG,
		},
		{
			name:  "binary.to_bytes",
			body:  "binary.to_bytes(x)",
			want:  "binary.to_bytes got \"2\": not a bit string",
			short: `"2"`,
			long:  BIG,
		},
		{
			name:  "base64.decode",
			body:  "base64.decode(x)",
			want:  "base64.decode got \"!\": not the encoding this reads",
			short: `"!"`,
			long:  `"!" * 5000000`,
		},
		{
			name:  "base32.decode",
			body:  "base32.decode(x)",
			want:  "base32.decode got \"!\": not the encoding this reads",
			short: `"!"`,
			long:  `"!" * 5000000`,
		},
		{
			name:  "integer.to_bytes order",
			body:  "integer.to_bytes(1, 2, x)",
			want:  "integer.to_bytes got \"zz\": not a byte order",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name: "regexp pattern",
			body: "regexp.match(x, 'a')",
			want: "regexp.match got \"(\": not a pattern this reads: error parsi" +
				"ng regexp: missing closing ): `(`",
			short: `"("`,
			long:  `"(" + "a" * 5000000`,
		},
		{
			name:  "hash.hmac algorithm",
			body:  "hash.hmac(x, b'k', b'd')",
			want:  "hash.hmac: \"zz\": no such algorithm",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name: "state.set value",
			body: "state.set(x, [buf.new()])",
			want: "state.set \"zz\": buffer: not data a store can hold: " +
				"value cannot be converted",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name: "event.post value",
			body: "event.post(x, buf.new())",
			want: "event.post \"zz\": buffer: not data an event can carry: " +
				"value cannot be converted",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name:  "jsonpath no slash",
			body:  "extract_json({}, x)",
			want:  "extract_json: \"a\" does not start with \"/\": not a json pointer",
			short: `"a"`,
			long:  BIG,
		},
		{
			name: "jsonpath not an index",
			body: "extract_json([1], '/' + x)",
			want: "extract_json: \"/zz\": \"zz\" is not an index: not a json poi" +
				"nter",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath operation",
			body: "patch_json([1], [{'op': x}])",
			want: "patch_json: operation 0: \"zz\": unknown patch operation: " +
				"malformed patch operation",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name: "jsonpath move into itself",
			body: "patch_json({}, [{'op': 'move', 'from': '/' + x, 'path': '/' +" +
				" x + '/b'}])",
			want: "patch_json: operation 0: \"/a\" into \"/a/b\": cannot move a " +
				"value into itself",
			short:  `"a"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath test op",
			body: "patch_json({x: 1}, [{'op': 'test', 'path': '/' + x, 'value': " +
				"2}])",
			want:   "patch_json: operation 0: /a is 1, not 2: test failed",
			short:  `"a"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath add",
			body: "patch_json([1], [{'op': 'add', 'path': '/' + x, 'value': 1}])",
			want: "patch_json: operation 0: \"zz\" is not an index: not a json p" +
				"ointer",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath replace",
			body: "patch_json([1], [{'op': 'replace', 'path': '/' + x, 'value': " +
				"1}])",
			want: "patch_json: operation 0: \"/zz\": \"zz\" is not an index: not" +
				" a json pointer",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath remove",
			body: "patch_json([1], [{'op': 'remove', 'path': '/' + x}])",
			want: "patch_json: operation 0: \"/zz\": \"zz\" is not an index: not" +
				" a json pointer",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name:  "negative integer",
			body:  "hex.from_int(-x, 4)",
			want:  "hex.from_int got -5, which is negative: out of range",
			short: `5`,
			long:  NUMBER,
		},
		{
			name:  "integer too wide",
			body:  "integer.to_bytes(x, 2)",
			want:  "integer.to_bytes: 70000 does not fit 2 bytes: out of range",
			short: `70000`,
			long:  NUMBER,
		},
		{
			name:  "negative integer to_bytes",
			body:  "integer.to_bytes(-x, 2)",
			want:  "integer.to_bytes got -5, which is negative: out of range",
			short: `5`,
			long:  NUMBER,
		},
		{
			name:  "crc32 seed",
			body:  "hash.crc32(b'x', x)",
			want:  "hash.crc32 got 4294967296 to continue: out of range",
			short: `4294967296`,
			long:  NUMBER,
		},
		{
			name:  "file.read path",
			body:  "file.read(x)",
			want:  "file.read: stat /nonexistent/zz: no such file or directory",
			short: `"/nonexistent/zz"`,
			long:  `"/" + "z" * 1000000`,
		},
		{
			name: "jsonpath add into nothing",
			body: "patch_json({'a': {}}, [{'op': 'add', 'path': '/a/' + x + '/b'" +
				", 'value': 1}])",
			want:  "patch_json: operation 0: \"zz\": no such path",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name:   "jsonpath remove from nothing",
			body:   "patch_json({}, [{'op': 'remove', 'path': '/' + x + '/b'}])",
			want:   "patch_json: operation 0: \"/zz\": no such path",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath replace in nothing",
			body: "patch_json({}, [{'op': 'replace', 'path': '/' + x + '/b', 'va" +
				"lue': 1}])",
			want:   "patch_json: operation 0: \"/zz\": no such path",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "jsonpath step under an index",
			body: "patch_json([[1]], [{'op': 'replace', 'path': '/0/' + x, 'valu" +
				"e': 1}])",
			want: "patch_json: operation 0: \"/0/zz\": \"zz\" is not an index: n" +
				"ot a json pointer",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name:  "event.wait duration",
			body:  "event.wait(x, 'bad')",
			want:  "event.wait \"zz\": got string: not a duration",
			short: `"zz"`,
			long:  BIG,
		},
		{
			name: "state.update value",
			body: "state.update(x, lambda s: buf.new())",
			want: "state.update \"zz\": buffer: not data a store can hold: " +
				"value cannot be converted",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "join inside an update",
			body: "state.update(x, lambda s: join(spawn(lambda: 1)))",
			want: "state.update \"zz\": join inside an update of \"zz\": an upda" +
				"te cannot start another",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "event.wait inside an update",
			body: "state.update(x, lambda s: event.wait('e', 1))",
			want: "state.update \"zz\": event.wait inside an update of \"zz\": a" +
				"n update cannot start another",
			short:  `"zz"`,
			long:   BIG,
			quotes: 2,
		},
		{
			name: "nested update",
			body: "state.update(x, lambda s: state.update(x, lambda t: 1))",
			want: "state.update \"zz\": state.update \"zz\": already updating \"" +
				"zz\": an update cannot start another",
			short:  `"zz"`,
			long:   BIG,
			quotes: 3,
		},
		{
			name: "file.writelines refused path",
			body: "file.writelines(x, [1])",
			want: "file.writelines: open /tmp/lark-q4-never/zz: no such file or " +
				"directory",
			short: `"/tmp/lark-q4-never/zz"`,
			long:  BIG,
		},
		{
			name: "arg name",
			top:  "V = arg(X)",
			want: "initialise probe.star: zz: argument not supplied and has no d" +
				"efault (supply it as -args '{\"zz\": ...}', or give arg() a " +
				"default)",
			short: `"zz"`,
			long:  huge,
		},
		{
			name: "load target",
			top:  "load(X, 'a')",
			want: "cannot load \"zz\" from probe.star: open zz: no such file or " +
				"directory",
			short: `"zz"`,
			long:  huge,
		},
	}
}

// TestErrors_ShortInputKeepsTodaysText pins the whole error of every site
// for an input of common.QUOTE_LIMIT bytes or fewer, as the code wrote it before
// change C21.
//
// Revisions:
//   - 2026-11-02 11:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestErrors_ShortInputKeepsTodaysText(t *testing.T) {
	for _, site := range _Sites() {
		t.Run(
			site.name,
			func(t *testing.T) {
				got := _RunSite(
					t,
					&site,
					site.short,
				)
				if got != site.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						site.want,
					)
				}
			},
		)
	}
}

// TestErrors_LongInputIsBounded gives every site an input that is large,
// and expects an error of the size the helper bounds: each quoted piece is at
// most 4*QUOTE_LIMIT+2 bytes plus the marker, and the fixed text is short.
//
// Revisions:
//   - 2026-11-02 11:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestErrors_LongInputIsBounded(t *testing.T) {
	for _, site := range _Sites() {
		if site.long == "" {
			continue
		}

		t.Run(
			site.name,
			func(t *testing.T) {
				got := _RunSite(
					t,
					&site,
					site.long,
				)

				var (
					quotes = max(
						site.quotes,
						1,
					)
					piece = 4*common.QUOTE_LIMIT + 2 + len("... ( bytes)") + 10
					limit = quotes*piece + 400
				)

				if len(got) > limit {
					t.Fatalf(
						"error is %d bytes, want at most %d",
						len(got),
						limit,
					)
				}

				if !utf8.ValidString(got) || strings.ContainsAny(
					got,
					"\n\r",
				) {
					t.Fatalf(
						"error is not one clean line: %.300q",
						got,
					)
				}
			},
		)
	}
}
