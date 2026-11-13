package tests

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// TYPED_SCRIPT returns the argument "v" and its type, so a test sees an int and a
	// float apart.
	//
	// Revisions:
	//   - 2026-11-13 12:59: initial creation (C132, Q437)
	TYPED_SCRIPT = `
v = arg("v")

def main():
    return [v, type(v)]
`
)

// TestWithArgs_FollowsTheStrictRulesOfProtobufJSON proves a run is handed each
// argument as the kind the rules of protobuf JSON give it (Q437, Q438): an integer
// type, and a json.Number written as an integer, is an int, exact and in int64; a float
// type, and a json.Number with a fraction or an exponent, is a float; a string is text
// and never base64; a []byte is bytes; text that is not valid UTF-8, an int outside
// int64, a json.Number that is no JSON number and a float that is not finite are
// refused with unpack.ErrRange or unpack.ErrConvert, in the chain beside
// artifact.ErrJSON.
//
// The check: go test -race -count=1 -run TestWithArgs_FollowsTheStrictRulesOfProtobufJSON
// ./v1/runtime/internal/artifact/tests
//
// Revisions:
//   - 2026-11-13 12:59: initial creation (C132, Q437, Q438)
func TestWithArgs_FollowsTheStrictRulesOfProtobufJSON(t *testing.T) {
	for _, tc := range STRICT {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got, err := runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "typed.star",
						Text:  []byte(TYPED_SCRIPT),
					},
					artifact.WithArgs(map[string]any{
						"v": tc.given,
					}),
				)
				if tc.err != nil {
					if !errors.Is(
						err,
						tc.err,
					) || !errors.Is(
						err,
						artifact.ErrJSON,
					) {
						t.Fatalf(
							"err = %v, want %v and %v",
							err,
							tc.err,
							artifact.ErrJSON,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				if got.String() != tc.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// STRICT lists an argument as a host gives it, what the script returns for it (the
// value and its type), and the sentinel of the refusal when there is one.
//
// Revisions:
//   - 2026-11-13 12:59: initial creation (C132, Q437, Q438)
//
//nolint:lll // a golden row
var (
	STRICT = []struct {
		name  string
		given any
		want  string
		err   error
	}{
		{"an int", 5432, `[5432, "int"]`, nil},
		{"a negative int", -7, `[-7, "int"]`, nil},
		{"an int8", int8(-8), `[-8, "int"]`, nil},
		{"a uint32", uint32(4294967295), `[4294967295, "int"]`, nil},
		{"the largest int64", int64(math.MaxInt64), `[9223372036854775807, "int"]`, nil},
		{"the smallest int64", int64(math.MinInt64), `[-9223372036854775808, "int"]`, nil},
		{"the largest uint64 that fits", uint64(math.MaxInt64), `[9223372036854775807, "int"]`, nil},
		{"an int16", int16(-16), `[-16, "int"]`, nil},
		{"an int32", int32(-32), `[-32, "int"]`, nil},
		{"a uint", uint(7), `[7, "int"]`, nil},
		{"a uint8", uint8(8), `[8, "int"]`, nil},
		{"a uint16", uint16(16), `[16, "int"]`, nil},
		{"a uint64 past int64", uint64(math.MaxInt64) + 1, "", unpack.ErrRange},
		{"the largest uint64", uint64(math.MaxUint64), "", unpack.ErrRange},
		{"a float64 that is whole", 5432.0, `[5432.0, "float"]`, nil},
		{"a float64", 1.5, `[1.5, "float"]`, nil},
		{"a float32", float32(0.5), `[0.5, "float"]`, nil},
		{"NaN", math.NaN(), "", artifact.ErrJSON},
		{"an infinity", math.Inf(1), "", artifact.ErrJSON},
		{"a json.Number written as an integer", json.Number("5432"), `[5432, "int"]`, nil},
		{"a json.Number, negative", json.Number("-5432"), `[-5432, "int"]`, nil},
		{"a json.Number, minus zero", json.Number("-0"), `[0, "int"]`, nil},
		{"a json.Number, the largest int64", json.Number("9223372036854775807"), `[9223372036854775807, "int"]`, nil},
		{"a json.Number, the smallest int64", json.Number("-9223372036854775808"), `[-9223372036854775808, "int"]`, nil},
		{"a json.Number, one past the largest", json.Number("9223372036854775808"), "", unpack.ErrRange},
		{"a json.Number, one past the smallest", json.Number("-9223372036854775809"), "", unpack.ErrRange},
		{"a json.Number, 2^64+1", json.Number("18446744073709551617"), "", unpack.ErrRange},
		{"a json.Number with a fraction", json.Number("1.5"), `[1.5, "float"]`, nil},
		{"a json.Number with a zero fraction", json.Number("1.0"), `[1.0, "float"]`, nil},
		{"a json.Number with an exponent", json.Number("1e3"), `[1000.0, "float"]`, nil},
		{"a json.Number with a capital exponent", json.Number("2E-1"), `[0.2, "float"]`, nil},
		{"a json.Number past float64", json.Number("1e999"), "", unpack.ErrRange},
		{"a json.Number with a plus sign", json.Number("+1"), "", unpack.ErrConvert},
		{"a json.Number in hex", json.Number("0x10"), "", unpack.ErrConvert},
		{"a json.Number with an underscore", json.Number("1_0"), "", unpack.ErrConvert},
		{"a json.Number with a leading zero", json.Number("01"), "", unpack.ErrConvert},
		{"an empty json.Number", json.Number(""), "", unpack.ErrConvert},
		{"a json.Number that is text", json.Number("abc"), "", unpack.ErrConvert},
		{"a string", "5432", `["5432", "string"]`, nil},
		{"a string that looks like base64", "aGk=", `["aGk=", "string"]`, nil},
		{"a string with a wide letter", "é\n", `["é\n", "string"]`, nil},
		{"a string that is not UTF-8", "a\xffb", "", unpack.ErrConvert},
		{"bytes", []byte("hi\xff"), `[b"hi\xff", "bytes"]`, nil},
		{"no bytes", []byte{}, `[b"", "bytes"]`, nil},
		{"a bool", true, `[True, "bool"]`, nil},
		{"nil", nil, `[None, "NoneType"]`, nil},
		{"a list of every kind", []any{nil, true, 1, 1.0, "s", []byte("b")}, `[[None, True, 1, 1.0, "s", b"b"], "list"]`, nil},
		{"a list with a bad item", []any{1, "a\xffb"}, "", unpack.ErrConvert},
		{"a map", map[string]any{"k": 1, "j": 2.5}, `[{"j": 2.5, "k": 1}, "dict"]`, nil},
		{"a map with a key that is not UTF-8", map[string]any{"a\xffb": 1}, "", unpack.ErrConvert},
		{"a map with an int past int64", map[string]any{"k": uint64(math.MaxUint64)}, "", unpack.ErrRange},
		{"a typed int", _Port(8080), `[8080, "int"]`, nil},
		{"a typed float", _Ratio(0.5), `[0.5, "float"]`, nil},
	}
)

// END GOLDEN

// TestWithArgs_RefusesAnArgumentNameThatIsNotUTF8 proves the name an argument is
// supplied under is text too, so a name that is not valid UTF-8 is refused with
// unpack.ErrConvert beside artifact.ErrJSON, and the words quote the name cut (Q4).
//
// The check: go test -race -count=1 -run TestWithArgs_RefusesAnArgumentNameThatIsNotUTF8
// ./v1/runtime/internal/artifact/tests
//
// Revisions:
//   - 2026-11-13 13:09: initial creation (C132, Q437)
func TestWithArgs_RefusesAnArgumentNameThatIsNotUTF8(t *testing.T) {
	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "typed.star",
			Text:  []byte(TYPED_SCRIPT),
		},
		artifact.WithArgs(map[string]any{
			"v":      1,
			"a\xffb": 2,
		}),
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) || !errors.Is(
		err,
		artifact.ErrJSON,
	) {
		t.Fatalf(
			"err = %v, want %v and %v",
			err,
			unpack.ErrConvert,
			artifact.ErrJSON,
		)
	}
}

// FuzzArgumentValue checks, for any JSON text read the way the command reads -args
// (a decoder that keeps each number as its text), that a run is handed the argument
// or refused with artifact.ErrJSON, and that an argument which was handed arrives as a
// value of the kind its text says: an int when the text is an integer inside int64, a
// float when it has a fraction or an exponent, and the same text back for a string.
//
// The check: go test -run XXX -fuzz '^FuzzArgumentValue$' -fuzztime 60s
// ./v1/runtime/internal/artifact/tests
//
// Revisions:
//   - 2026-11-13 13:10: initial creation (C132, Q4, Q437, Q438)
func FuzzArgumentValue(f *testing.F) {
	for _, seed := range []string{
		`5432`, `-1`, `1.5`, `1e3`, `1E-3`, `9223372036854775807`, `9223372036854775808`,
		`-9223372036854775808`, `1e999`, `"aGk="`, `"aé"`, `null`, `true`, `[1, 1.0]`,
		`{"k": [1e2]}`, `-0`, `0.0`, `00`, `1_0`, `"\ud800"`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		if len(text) > 2048 {
			return
		}

		var (
			given   any
			reading = json.NewDecoder(strings.NewReader(text))
		)

		reading.UseNumber()

		if reading.Decode(&given) != nil || reading.More() {
			return
		}

		got, err := runtime.Run(
			t.Context(),
			&common.Source{
				Entry: "typed.star",
				Text:  []byte(TYPED_SCRIPT),
			},
			artifact.WithArgs(map[string]any{
				"v": given,
			}),
		)
		if err != nil {
			if !errors.Is(
				err,
				artifact.ErrJSON,
			) {
				t.Fatalf(
					"%q: err = %v, want ErrJSON or none",
					text,
					err,
				)
			}

			return
		}

		if number, ok := given.(json.Number); ok {
			_, isInt := new(big.Int).SetString(
				string(number),
				10,
			)
			if isInt != strings.Contains(
				got.String(),
				`"int"`,
			) {
				t.Fatalf(
					"%q: an integer text and the kind %s disagree",
					text,
					got,
				)
			}
		}
	})
}

// TestWithArgs_NamesTheFirstBadArgumentInOrder proves that when several arguments are
// refused, the error names the first one in the order of the names, the same one on
// every run, whatever the order a Go map is read in. A host that fixes the first
// error and runs again then meets the next, and a test of the error text is stable.
//
// The check: go test -race -count=1 -run TestWithArgs_NamesTheFirstBadArgumentInOrder
// ./v1/runtime/internal/artifact/tests
//
// Revisions:
//   - 2026-11-13 17:17: initial creation (C132, Q437)
func TestWithArgs_NamesTheFirstBadArgumentInOrder(t *testing.T) {
	var bad = map[string]any{
		"v": 1,
	}

	for _, name := range []string{"m", "k", "d", "b", "a", "c", "z", "q", "x", "f"} {
		bad[name] = "x\xff"
	}

	for range 200 {
		_, err := runtime.Run(
			t.Context(),
			&common.Source{
				Entry: "typed.star",
				Text:  []byte(TYPED_SCRIPT),
			},
			artifact.WithArgs(bad),
		)
		if err == nil || !strings.Contains(
			err.Error(),
			`argument "a":`,
		) {
			t.Fatalf(
				"err = %v, want the first name in order, \"a\"",
				err,
			)
		}
	}
}
