// Regression probes for what a spawned thread is handed: a lambda written
// inside spawn takes the variables it reads at the spawn, frozen, and anything
// else that would carry a local variable into a thread is refused.
package testing_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
)

// _Case is one function in testdata/capture.star, and what calling it gives:
// a value, a sentinel it fails with, or text its failure contains.
type _Case struct {
	fn    string
	value string
	err   error
	text  string
}

const (
	CAPTURE = "testdata/capture.star"
	FROZEN  = "frozen"
	BEFORE  = "referenced before assignment"
)

// CASES is every case in the fixture, in the order it declares them.
var CASES = []*_Case{
	{fn: "plain", value: `["done"]`},
	{fn: "with_arguments", value: `["hello, alice"]`},
	{fn: "generated", value: `["hi, bob"]`},
	{fn: "fanout", value: `[0, 10, 20, 30, 40]`},
	{fn: "many", value: `200`},
	{fn: "reused", value: `["HELLO", "WORLD"]`},
	{fn: "dag", value: `[["a", "b"], ["b"], ["a"]]`},
	{fn: "shared_read", value: `[8080, 8080]`},
	{fn: "collected", value: `[1, 1]`},
	{fn: "wrapped", value: `"fetched archive"`},
	{fn: "elsewhere", value: `"total 6"`},
	{fn: "loaded", value: `["HELLO"]`},
	{fn: "shared_write", text: FROZEN},
	{fn: "main_writes", text: FROZEN},
	{fn: "too_early", text: BEFORE},
	{fn: "by_name", err: runtime.ErrCaptures},
	{fn: "made_earlier", err: runtime.ErrCaptures},
	{fn: "closure_taken", err: runtime.ErrCaptures},
}

// TestCapture_EveryCase runs each case in the fixture through the compiler and
// the runtime a host uses, under -race like the rest of the suite.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func TestCapture_EveryCase(t *testing.T) {
	src, err := os.ReadFile(CAPTURE)
	if err != nil {
		t.Fatal(err)
	}

	built, err := runtime.NewCompiler().Compile(CAPTURE, src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	for _, tc := range CASES {
		t.Run(tc.fn, func(t *testing.T) {
			value, err := built.Invoke(t.Context(), tc.fn)

			got := ""
			if err == nil {
				got = value.String()
			}

			_Expected(t, tc, got, err)
		})
	}
}

// _Expected fails unless an outcome is what a case says.
//
// Revisions:
//   - 2026-09-29 23:33: initial creation
func _Expected(t *testing.T, tc *_Case, got string, err error) {
	t.Helper()

	switch {
	case tc.err != nil:
		if !errors.Is(err, tc.err) {
			t.Fatalf("got %q and %v, want %v", got, err, tc.err)
		}

	case tc.text != "":
		if err == nil || !strings.Contains(err.Error(), tc.text) {
			t.Fatalf("got %q and %v, want a failure saying %q", got, err, tc.text)
		}

	default:
		if err != nil || got != tc.value {
			t.Fatalf("got %q and %v, want %s", got, err, tc.value)
		}
	}
}
