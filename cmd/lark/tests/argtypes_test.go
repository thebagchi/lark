package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
)

const (
	// TYPED_ARGS declares the arguments the -args tests give, and prints each with its type.
	//
	// Revisions:
	//   - 2026-11-13 13:01: initial creation (C132, Q437)
	TYPED_ARGS = `
port = arg("port", 0)
r = arg("r", 0)
n = arg("n", 0)
name = arg("name", "")

def main():
    print(port, type(port), r, type(r), n, type(n), name, type(name))
`
)

// TestArgs_ReadEachNumberByItsText proves, from the command line, that a number in
// -args written as an integer is an exact int, one with a fraction or an exponent is a
// float, an int outside int64 is refused with its words short, a string is a string,
// the same holds when a key nothing declares is dropped with a warning, and a flow
// runs the same as the script (Q437, Q438).
//
// The check: go test -race -count=1 -run TestArgs_ReadEachNumberByItsText
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 13:01: initial creation (C132, Q437, Q438)
func TestArgs_ReadEachNumberByItsText(t *testing.T) {
	var (
		dir    = t.TempDir()
		script = filepath.Join(
			dir,
			"typed.star",
		)
	)

	err := os.WriteFile(
		script,
		[]byte(TYPED_ARGS),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	code, flow, stderr := _Said(
		t,
		"convert",
		script,
	)
	if code != 0 {
		t.Fatalf(
			"convert: exit %d (stderr %q)",
			code,
			stderr,
		)
	}

	var json = filepath.Join(
		dir,
		"typed.json",
	)

	err = os.WriteFile(
		json,
		[]byte(flow),
		SHARED,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args string
		code int
		has  string
	}{
		{
			"an integer",
			`{"port": 5432}`,
			0,
			`5432 int 0 int`,
		},
		{
			"a number with a fraction",
			`{"r": 1.5}`,
			0,
			`0 int 1.5 float`,
		},
		{
			"a number with a zero fraction",
			`{"r": 1.0}`,
			0,
			`1.0 float`,
		},
		{
			"a number with an exponent",
			`{"r": 1e3}`,
			0,
			`1000.0 float`,
		},
		{
			"the largest int64",
			`{"n": 9223372036854775807}`,
			0,
			`9223372036854775807 int`,
		},
		{
			"the smallest int64",
			`{"n": -9223372036854775808}`,
			0,
			`-9223372036854775808 int`,
		},
		{
			"one past the largest",
			`{"n": 9223372036854775808}`,
			1,
			`out of range`,
		},
		{
			"a long integer",
			`{"n": ` + strings.Repeat(
				"9",
				5000,
			) + `}`,
			1,
			`out of range`,
		},
		{
			"an integer beside a key nothing declares",
			`{"port": 5432, "colour": "red"}`,
			0,
			`5432 int`,
		},
		{
			"a float beside a key nothing declares",
			`{"r": 1e3, "colour": "red"}`,
			0,
			`1000.0 float`,
		},
		{
			"a long integer beside a key nothing declares",
			`{"colour": 1, "n": ` + strings.Repeat(
				"9",
				5000,
			) + `}`,
			1,
			`out of range`,
		},
		{
			"a string that looks like base64",
			`{"name": "aGk="}`,
			0,
			`aGk= string`,
		},
		{
			"a string that looks like a number",
			`{"name": "5432"}`,
			0,
			`5432 string`,
		},
	} {
		for _, file := range []string{script, json} {
			t.Run(
				tc.name+" "+filepath.Ext(file),
				func(t *testing.T) {
					code, printed, stderr := _Said(
						t,
						"run",
						"-args",
						tc.args,
						file,
					)
					if code != tc.code {
						t.Fatalf(
							"exit %d, want %d (stderr %.300q)",
							code,
							tc.code,
							stderr,
						)
					}

					if !strings.Contains(
						printed+stderr,
						tc.has,
					) {
						t.Fatalf(
							"output %.300q lacks %q",
							printed+stderr,
							tc.has,
						)
					}

					if len(stderr) > 600 {
						t.Fatalf(
							"stderr is %d bytes",
							len(stderr),
						)
					}
				},
			)
		}
	}
}

// TestArguments_KeepsEachNumberAsText proves command.Arguments reads the -args JSON
// into the map a run is handed with each number as the text it was written in, so an
// int past float64's exact range keeps its digits, and that text which is not an
// object is refused with the words of encoding/json (Q4, Q437).
//
// The check: go test -race -count=1 -run TestArguments_KeepsEachNumberAsText
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 13:25: initial creation (C132, Q4, Q437)
func TestArguments_KeepsEachNumberAsText(t *testing.T) {
	got, err := command.Arguments(
		`{"a": 9007199254740993, "b": 1.50, "c": [1e3], "d": "x", "e": null}`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got["a"] != json.Number("9007199254740993") ||
		got["b"] != json.Number("1.50") ||
		got["d"] != "x" || got["e"] != nil {
		t.Fatalf(
			"got %#v",
			got,
		)
	}

	list, _ := got["c"].([]any)
	if len(list) != 1 || list[0] != json.Number("1e3") {
		t.Fatalf(
			"got %#v",
			got["c"],
		)
	}

	for _, bad := range []string{
		`{bad`,
		`[1, 2]`,
		`"x"`,
		`{"a": 1} {"b": 2}`,
		``,
	} {
		_, err := command.Arguments(bad)
		if err == nil {
			t.Fatalf(
				"%q: want an error",
				bad,
			)
		}
	}

	null, err := command.Arguments(`null`)
	if err != nil || null != nil {
		t.Fatalf(
			"null: got %v, %v",
			null,
			err,
		)
	}
}
