package tests

import (
	"errors"
	"path"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/args"
)

const (
	// LONE_CR_ARG is a script whose lines end with a lone carriage return and
	// whose argument nothing reads.
	//
	// Revisions:
	//   - 2026-11-12 23:41: initial creation (C130, Q405)
	LONE_CR_ARG = "x = arg(\"a\")\rdef main():\r    print(\"odd\")\r"

	// LONE_CR_LOADER is a script with lone carriage returns that loads a function
	// of LONE_CR_MODULE.
	//
	// Revisions:
	//   - 2026-11-12 23:52: initial creation (C130, Q405)
	LONE_CR_LOADER = "load(\"module.star\", \"f\")\rdef main():\r    return f()\r"

	// LONE_CR_MODULE is a module with lone carriage returns and an argument
	// nothing reads.
	//
	// Revisions:
	//   - 2026-11-12 23:41: initial creation (C130, Q405)
	LONE_CR_MODULE = "m = arg(\"b\")\rdef f():\r    return 1\r"
)

// _Carriage is a Loader that serves LONE_CR_MODULE as module.star.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
type _Carriage struct{}

// Resolve reads target as a name beside from.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func (m *_Carriage) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns LONE_CR_MODULE.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func (m *_Carriage) Load(string) ([]byte, error) {
	return []byte(LONE_CR_MODULE), nil
}

// TestRun_ALoneCarriageReturnScriptIsShakenAsAnyOther proves a script whose line
// breaks are lone carriage returns is shaken as one with line feeds: an argument
// nothing reads is not required of the run, the flow derived from the script
// lists none, and the flow runs, so the script and the flow agree (Q405). A
// module the script loads is read the same way.
//
// The check: go test -race -count=1 -run TestRun_ALoneCarriageReturnScriptIsShakenAsAnyOther
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func TestRun_ALoneCarriageReturnScriptIsShakenAsAnyOther(t *testing.T) {
	var tests = []struct {
		name   string
		source *common.Source
	}{
		{
			"a script",
			&common.Source{
				Entry: "odd.star",
				Text:  []byte(LONE_CR_ARG),
			},
		},
		{
			"a script and a module",
			&common.Source{
				Entry:  "odd.star",
				Text:   []byte(LONE_CR_LOADER),
				Loader: &_Carriage{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Run(
					t.Context(),
					tc.source,
				)
				if err != nil {
					t.Fatalf(
						"run the script: %v",
						err,
					)
				}

				flow, err := runtime.Derive(tc.source)
				if err != nil {
					t.Fatal(err)
				}

				if len(flow.GetArgs()) > 0 {
					t.Fatalf(
						"the flow lists arguments: %v",
						flow.GetArgs(),
					)
				}

				text, err := runtime.Emit(flow)
				if err != nil {
					t.Fatal(err)
				}

				_, err = runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "odd_flow.star",
						Text:  text,
					},
				)
				if err != nil {
					t.Fatalf(
						"run the flow: %v",
						err,
					)
				}
			},
		)
	}
}

// TestRun_ALoneCarriageReturnGivesTheErrorsOfLineFeeds proves a lone carriage
// return changes no line number: a script fails with the same words, and the
// same line and column, as the one with line feeds in its place, for a failure
// at run time, a parse error and a resolve error, in a script and in a module.
// The scanner reads both as one line break, and Normal writes the same (Q405).
//
// The check: go test -race -count=1 -run TestRun_ALoneCarriageReturnGivesTheErrorsOfLineFeeds
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func TestRun_ALoneCarriageReturnGivesTheErrorsOfLineFeeds(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"a failure at run time",
			"x = 1\ny = 2\ndef main():\n    fail(\"boom\")\n",
		},
		{
			"an undefined name",
			"x = 1\n\n\ndef main():\n    return z\n",
		},
		{
			"a parse error",
			"x = 1\ny = (2\n\n",
		},
		{
			"an error after a string over lines",
			"s = '''a\nb\nc'''\ndef main():\n    return q\n",
		},
		{
			"an error after a comment",
			"# c\nx = 1  # d\ndef main():\n    return q\n",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				want := _Broken(
					t,
					tc.text,
				)
				if want == "" {
					t.Fatal("the script with line feeds did not fail")
				}

				got := _Broken(
					t,
					strings.ReplaceAll(
						tc.text,
						"\n",
						"\r",
					),
				)
				if got != want {
					t.Fatalf(
						"error: got %q, want %q",
						got,
						want,
					)
				}
			},
		)
	}
}

// _Broken is the words a run of text fails with.
//
// Revisions:
//   - 2026-11-12 23:41: initial creation (C130, Q405)
func _Broken(
	t *testing.T,
	text string,
) string {
	t.Helper()

	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "fails.star",
			Text:  []byte(text),
		},
	)
	if err == nil {
		return ""
	}

	return err.Error()
}

// TestRun_ANulScriptIsKeptWholeByTheCompileAndShakenByTheFlow proves Q379 for a
// NUL on the shake side: a NUL ends the file for the parser, so the compile keeps
// a script that holds one whole, and an argument nothing reads is still asked
// for, where the flow derived from it drops the argument and runs with none. A
// lone carriage return is no longer odd, so a NUL is the one text that splits
// the two (Q405).
//
// The check: go test -race -count=1 -run TestRun_ANulScript ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 00:45: initial creation (C130, Q379, Q405)
func TestRun_ANulScriptIsKeptWholeByTheCompileAndShakenByTheFlow(t *testing.T) {
	var source = &common.Source{
		Entry: "nul.star",
		Text:  []byte("x = arg(\"a\")\ndef main():\n    print(\"nul\")\n# c\x00 junk\n"),
	}

	_, err := runtime.Run(
		t.Context(),
		source,
	)
	if !errors.Is(
		err,
		args.ErrRequired,
	) {
		t.Fatalf(
			"run the script: err = %v, want ErrRequired",
			err,
		)
	}

	flow, err := runtime.Derive(source)
	if err != nil {
		t.Fatal(err)
	}

	if len(flow.GetArgs()) > 0 {
		t.Fatalf(
			"the flow lists arguments: %v",
			flow.GetArgs(),
		)
	}

	text, err := runtime.Emit(flow)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "nul_flow.star",
			Text:  text,
		},
	)
	if err != nil {
		t.Fatalf(
			"run the flow: %v",
			err,
		)
	}
}
