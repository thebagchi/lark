package tests

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	// TAGGED is the hint that goes with a refusal of a flow of plain values (Q433).
	//
	// Revisions:
	//   - 2026-11-13 17:42: initial creation (C133, Q433, Q444)
	TAGGED = "(a value is an object named for its kind, " +
		"such as {\"string\": \"x\"} or {\"int\": \"3\"})"

	// NULLED is the hint that goes with a refusal of a null where a value stands (Q442).
	//
	// Revisions:
	//   - 2026-11-13 17:42: initial creation (C133, Q442, Q444)
	NULLED = "(a value is an object named for its kind, such as {\"null\": null})"

	// BOUND is the most bytes of an error from Read that these tests accept when the
	// input holds long untrusted text: 100 bytes of quote (Q4), and the fixed words
	// around it.
	//
	// Revisions:
	//   - 2026-11-13 17:42: initial creation (C133, Q4, Q444)
	BOUND = 400
)

// COUNTED finds the count of bytes that a cut quote ends with.
//
// Revisions:
//   - 2026-11-13 17:38: initial creation (C133, Q4)
var (
	COUNTED = regexp.MustCompile(`\((\d+) bytes\)`)
)

// LONG finds a run of the letter a that a test cuts a quote from.
//
// Revisions:
//   - 2026-11-13 17:39: initial creation (C133, Q4)
var (
	LONG = regexp.MustCompile(`a{10,}`)
)

// _Plainly is text with what differs between binaries taken out: protobuf-go puts a
// no-break space at random in its messages (two bytes, so the count of a cut quote
// moves by one, and the cut one letter), so the count is made to say N and a long run of
// the letter a is made to say <a>.
//
// Revisions:
//   - 2026-11-13 17:38: initial creation (C133, Q4)
func _Plainly(text string) string {
	text = strings.ReplaceAll(
		text,
		"\u00a0",
		" ",
	)

	text = COUNTED.ReplaceAllString(
		text,
		"(N bytes)",
	)

	return LONG.ReplaceAllString(
		text,
		"<a>",
	)
}

// READ_FLOW is the flow of the Read example: one function, one call.
//
// Revisions:
//   - 2026-11-13 17:50: initial creation (C133, Q444)
const (
	READ_FLOW = `
{
  "functions": [{"name": "fetch", "params": ["name"], "body": "return name"}],
  "args": {"port": {"name": "port", "default": {"int": "8080"}}},
  "constants": {"HOST": {"value": {"string": "lark"}}},
  "main": {"statement": [{"call": {"function": "fetch", "args": [{"string": "lark"}]}}]}
}
`
)

// TestRead_ReadsAValidFlow proves Read returns the flow the JSON holds, and that
// the flow is one Check accepts and Emit writes.
//
// The check: go test -race -count=1 -run TestRead_ReadsAValidFlow ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q444, Q445)
func TestRead_ReadsAValidFlow(t *testing.T) {
	flow, err := runtime.Read([]byte(READ_FLOW))
	if err != nil {
		t.Fatal(err)
	}

	var want = new(workflowpb.Flow)

	err = protojson.Unmarshal(
		[]byte(READ_FLOW),
		want,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !proto.Equal(
		flow,
		want,
	) {
		t.Fatalf(
			"Read gives %v, protojson gives %v",
			flow,
			want,
		)
	}

	err = runtime.Check(flow)
	if err != nil {
		t.Fatalf(
			"Read returned a flow that Check refuses: %v",
			err,
		)
	}

	text, err := runtime.Emit(flow)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(
		string(text),
		`fetch("lark")`,
	) {
		t.Fatalf(
			"the script lacks the call:\n%s",
			text,
		)
	}
}

// TestRead_ReadsWhatDeriveWrites proves a flow of Derive, written by protojson,
// reads back to an equal flow, for a script that holds every kind of value.
//
// The check: go test -race -count=1 -run TestRead_ReadsWhatDeriveWrites ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q444, Q445)
func TestRead_ReadsWhatDeriveWrites(t *testing.T) {
	for _, tc := range AGREED {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := runtime.Derive(&common.Source{
					Entry: "kinds.star",
					Text:  []byte(tc.script),
				})
				if err != nil {
					t.Fatalf(
						"derive: %v",
						err,
					)
				}

				sent, err := protojson.Marshal(flow)
				if err != nil {
					t.Fatal(err)
				}

				read, err := runtime.Read(sent)
				if err != nil {
					t.Fatalf(
						"read %s: %v",
						sent,
						err,
					)
				}

				if !proto.Equal(
					flow,
					read,
				) {
					t.Fatalf(
						"Read gives %v, Derive gives %v",
						read,
						flow,
					)
				}
			},
		)
	}
}

// TestRead_RefusesWithTheWordsOfTheDecoder proves Read refuses a flow the decoder
// refuses, with the decoder's words cut to 100 bytes (Q4), the hint for a flow of
// plain values only when the decoder met a token where an object stands (Q433), and
// the error matching runtime.ErrForm. The texts are the ones the lark command printed
// at 5d51bae, less the name of the file.
//
// The check: go test -race -count=1 -run TestRead_RefusesWithTheWordsOfTheDecoder
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q433, Q444)
func TestRead_RefusesWithTheWordsOfTheDecoder(t *testing.T) {
	for _, tc := range DECODER {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := runtime.Read([]byte(tc.flow))
				if err == nil {
					t.Fatal("want an error")
				}

				if flow != nil {
					t.Fatalf(
						"a flow with an error: %v",
						flow,
					)
				}

				if _Plainly(err.Error()) != tc.want {
					t.Fatalf(
						"got\n%s\nwant\n%s",
						err,
						tc.want,
					)
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}
			},
		)
	}
}

// TestRead_RefusesAnIntOutsideInt64Bounded proves a flow with an int outside int64,
// or one that is no whole number, is refused with words that stay short when the int
// is long (Q4), no hint, and an error that matches runtime.ErrForm.
//
// The check: go test -race -count=1 -run TestRead_RefusesAnIntOutsideInt64Bounded
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q438, Q440, Q444)
func TestRead_RefusesAnIntOutsideInt64Bounded(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{
			"one past the largest",
			`"9223372036854775808"`,
		},
		{
			"one past the smallest",
			`"-9223372036854775809"`,
		},
		{
			"a fraction",
			`"1.5"`,
		},
		{
			"script text",
			`"1); import_os("`,
		},
		{
			"five thousand digits",
			`"` + strings.Repeat(
				"7",
				5000,
			) + `"`,
		},
		{
			"five thousand digits as a number",
			strings.Repeat(
				"7",
				5000,
			),
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Read([]byte(
					`{"constants": {"X": {"value": ` +
						`{"int": ` + tc.value + `}}}, "text": "pass"}`,
				))
				if err == nil {
					t.Fatal("want an error")
				}

				if len(err.Error()) > BOUND {
					t.Fatalf(
						"the error is %d bytes, want at most %d",
						len(err.Error()),
						BOUND,
					)
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}

				if strings.Contains(
					err.Error(),
					TAGGED,
				) {
					t.Fatalf(
						"a bad int has a hint: %s",
						err,
					)
				}
			},
		)
	}
}

// TestRead_RefusesANullWhereAValueStands proves a JSON null where a value is expected
// is refused with the hint, whatever field holds the value, and that a null kind that
// holds anything but null is out of range (Q439, Q442). protojson reads such a null as
// no value, so an argument default that is null would otherwise read as no default. A
// null in a field that is no value, and the value {"null": null}, are read.
//
// The check: go test -race -count=1 -run TestRead_RefusesANullWhereAValueStands
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q439, Q442, Q444)
func TestRead_RefusesANullWhereAValueStands(t *testing.T) {
	for _, tc := range NULLS {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Read([]byte(tc.flow))
				if err == nil {
					t.Fatal("want an error")
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}

				if !strings.Contains(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"%q lacks %q",
						err,
						tc.want,
					)
				}

				if len(err.Error()) > BOUND {
					t.Fatalf(
						"the error is %d bytes",
						len(err.Error()),
					)
				}
			},
		)
	}

	for _, ok := range []string{
		`{"constants": {"K": {"value": {"null": null}}}, "text": "pass"}`,
		`{"args": {"a": {"name": "a", "default": {"null": null}}}, "text": "pass"}`,
		`{"functions": [{"name": "f", "body": "pass", "params": null}], "text": "pass"}`,
		`{"text": "pass", "init": null}`,
	} {
		_, err := runtime.Read([]byte(ok))
		if err != nil {
			t.Fatalf(
				"%s: %v",
				ok,
				err,
			)
		}
	}
}

// TestRead_RefusesTheTextOfANullExactly proves the words of the null refusals: the
// path of schema names, the reason, and the hint of Q442, for a bare null and for a
// null kind that holds more than null.
//
// The check: go test -race -count=1 -run TestRead_RefusesTheTextOfANullExactly
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q439, Q442, Q444)
func TestRead_RefusesTheTextOfANullExactly(t *testing.T) {
	for _, tc := range []struct{ flow, want string }{
		{
			`{"args": {"a": {"name": "a", "default": null}}, "text": "pass"}`,
			"flow.args{}.default: a null where a value stands " + NULLED,
		},
		{
			`{"constants": {"K": {"value": {"null": 5}}}, "text": "pass"}`,
			"flow.constants{}.value.null: null holds more than null, " +
				"which is out of range " + NULLED,
		},
	} {
		_, err := runtime.Read([]byte(tc.flow))
		if err == nil || err.Error() != tc.want {
			t.Fatalf(
				"got %v\nwant %s",
				err,
				tc.want,
			)
		}
	}
}

// TestRead_RefusesAFlowThatCheckRefuses proves Read runs Check (Q445): a flow that
// decodes and is no flow lark can run is refused, with the error Check gives and the
// sentinel Check names, so a flow Read returns is always one lark can run.
//
// The check: go test -race -count=1 -run TestRead_RefusesAFlowThatCheckRefuses
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q445)
func TestRead_RefusesAFlowThatCheckRefuses(t *testing.T) {
	for _, tc := range CHECKED {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var decoded = new(workflowpb.Flow)

				err := protojson.Unmarshal(
					[]byte(tc.flow),
					decoded,
				)
				if err != nil {
					t.Fatalf(
						"the flow must decode: %v",
						err,
					)
				}

				checked := runtime.Check(decoded)
				if checked == nil {
					t.Fatal("Check must refuse the flow")
				}

				flow, err := runtime.Read([]byte(tc.flow))
				if err == nil {
					t.Fatal("want an error")
				}

				if flow != nil {
					t.Fatalf(
						"a flow with an error: %v",
						flow,
					)
				}

				if err.Error() != checked.Error() || err.Error() != tc.want {
					t.Fatalf(
						"Read says %q, Check says %q, want %q",
						err,
						checked,
						tc.want,
					)
				}

				if !errors.Is(
					err,
					tc.is,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						tc.is,
					)
				}
			},
		)
	}
}

// TestRead_BoundsTheWordsOfCheck proves the error for a flow that decodes and fails Check
// quotes a name of 5,000 bytes cut to 100 bytes (Q4): a name in the flow is the host's
// untrusted text.
//
// The check: go test -race -count=1 -run TestRead_BoundsTheWordsOfCheck ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q445)
func TestRead_BoundsTheWordsOfCheck(t *testing.T) {
	var name = strings.Repeat(
		"n",
		5000,
	)

	for _, flow := range []string{
		`{"functions":[{"name":"` + name + `","body":"return 1"},` +
			`{"name":"` + name + `","body":"return 2"}],` +
			`"main":{"statement":[{"call":{"function":"` + name + `"}}]}}`,
		`{"main":{"statement":[{"call":{"function":"` + name + `"}}]}}`,
	} {
		_, err := runtime.Read([]byte(flow))
		if err == nil {
			t.Fatal("want an error")
		}

		if len(err.Error()) > BOUND {
			t.Fatalf(
				"the error is %d bytes, want at most %d",
				len(err.Error()),
				BOUND,
			)
		}
	}
}

// TestRead_RefusesNothingAndEmptyInput proves empty, nil and blank input are
// refused with an error and no flow, and that {} is the empty flow Check judges.
//
// The check: go test -race -count=1 -run TestRead_RefusesNothingAndEmptyInput
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q444)
func TestRead_RefusesNothingAndEmptyInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{
			"nil",
			nil,
		},
		{
			"empty",
			[]byte{},
		},
		{
			"blanks",
			[]byte(" \n\t "),
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := runtime.Read(tc.data)
				if err == nil {
					t.Fatal("want an error")
				}

				if flow != nil {
					t.Fatalf(
						"a flow with an error: %v",
						flow,
					)
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}
			},
		)
	}

	var (
		flow, err = runtime.Read([]byte(`{}`))
		checked   = runtime.Check(new(workflowpb.Flow))
	)

	if (err == nil) != (checked == nil) || (flow == nil) != (err != nil) {
		t.Fatalf(
			"Read of {} gives %v, %v and Check of the empty flow gives %v",
			flow,
			err,
			checked,
		)
	}
}

// TestRead_ReadsAHugeFlowAndRefusesAHugeToken proves a flow of 16 MiB reads, and that
// a 16 MiB token the decoder refuses gives an error of bounded size (Q4): the input
// is no bound on the error.
//
// The check: go test -race -count=1 -run TestRead_ReadsAHugeFlowAndRefusesAHugeToken
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q444)
func TestRead_ReadsAHugeFlowAndRefusesAHugeToken(t *testing.T) {
	const SIZE = 16 << 20

	var huge = strings.Repeat(
		"a",
		SIZE,
	)

	flow, err := runtime.Read([]byte(
		`{"constants": {"X": {"value": {"string": "` + huge + `"}}}, "text": "pass"}`,
	))
	if err != nil {
		t.Fatalf(
			"a flow of %d bytes: %v",
			SIZE,
			err,
		)
	}

	if got := len(flow.GetConstants()["X"].GetValue().GetString_()); got != SIZE {
		t.Fatalf(
			"the string has %d bytes, want %d",
			got,
			SIZE,
		)
	}

	_, err = runtime.Read([]byte(
		`{"constants": {"X": {"value": "` + huge + `"}}, "text": "pass"}`,
	))
	if err == nil {
		t.Fatal("want an error")
	}

	if len(err.Error()) > BOUND {
		t.Fatalf(
			"the error is %d bytes, want at most %d",
			len(err.Error()),
			BOUND,
		)
	}

	found := COUNTED.FindStringSubmatch(err.Error())

	if found == nil {
		t.Fatalf(
			"the error does not say how long the text is: %.200s",
			err,
		)
	}

	counted, _ := strconv.Atoi(found[1])
	if counted < SIZE {
		t.Fatalf(
			"the error says %d bytes, want at least %d",
			counted,
			SIZE,
		)
	}

	_, err = runtime.Read([]byte(
		`{"` + huge + `": 1}`,
	))
	if err == nil || len(err.Error()) > BOUND {
		t.Fatalf(
			"an unknown field of %d bytes: want an error of at most %d bytes",
			SIZE,
			BOUND,
		)
	}
}

// TestRead_BoundsTheDepthOfAFlow proves a flow nested deep is read when it is within
// the depth the decoder holds (protojson: 10,000 messages, 2 for each list), and
// refused with a bounded error when it is not. The walk for a null has no limit of its
// own: it reads the tokens of the JSON (Q2).
//
// The check: go test -race -count=1 -run TestRead_BoundsTheDepthOfAFlow ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q2, Q4, Q444)
func TestRead_BoundsTheDepthOfAFlow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		depth int
		want  string
	}{
		{
			"1,000 lists",
			1000,
			"",
		},
		{
			"3,000 lists",
			3000,
			"",
		},
		{
			"4,500 lists",
			4500,
			"",
		},
		{
			"4,998 lists, the deepest the decoder reads",
			4998,
			"",
		},
		{
			"4,999 lists, one past it",
			4999,
			"proto: exceeded max recursion depth",
		},
		{
			"20,000 lists",
			20000,
			"proto: exceeded max recursion depth",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var value = strings.Repeat(
					`{"list": {"items": [`,
					tc.depth,
				) + `{"null": null}` + strings.Repeat(
					`]}}`,
					tc.depth,
				)

				flow, err := runtime.Read([]byte(
					`{"constants": {"K": {"value": ` + value + `}}, ` +
						`"text": "pass"}`,
				))

				if tc.want == "" {
					if err != nil || flow == nil {
						t.Fatalf(
							"want a flow: %v",
							err,
						)
					}

					return
				}

				if err == nil || _Plainly(err.Error()) != tc.want {
					t.Fatalf(
						"got %v, want %s",
						err,
						tc.want,
					)
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}
			},
		)
	}
}

// TestRead_MatchesErrFormAndTheDecoderError proves the error for a flow that does not
// read names the flow runtime.ErrForm and holds the decoder's own error, so the cut
// text loses neither cause (CLAUDE.md 1.7).
//
// The check: go test -race -count=1 -run TestRead_MatchesErrFormAndTheDecoderError
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q444)
func TestRead_MatchesErrFormAndTheDecoderError(t *testing.T) {
	_, err := runtime.Read([]byte("not json\n"))
	if err == nil {
		t.Fatal("want an error")
	}

	if !errors.Is(
		err,
		runtime.ErrForm,
	) {
		t.Fatalf(
			"%v does not match %v",
			err,
			runtime.ErrForm,
		)
	}

	var decoded = new(workflowpb.Flow)

	cause := protojson.Unmarshal(
		[]byte("not json\n"),
		decoded,
	)

	var multi interface{ Unwrap() []error }

	if !errors.As(
		err,
		&multi,
	) || len(multi.Unwrap()) != 2 {
		t.Fatalf(
			"%v does not hold the decoder's error beside ErrForm",
			err,
		)
	}

	if multi.Unwrap()[1].Error() != cause.Error() {
		t.Fatalf(
			"the cause is %q, the decoder says %q",
			multi.Unwrap()[1],
			cause,
		)
	}
}

// TestRead_ReturnsAFlowThatChecksAndEmits proves what Read promises, which is what Check
// promises (Q445, Q449): a flow it returns passes Check and Emit writes it. It does
// not promise a compile. A name that a text reads and nothing declares passes Read and
// Emit, and Compile refuses the script.
//
// The check: go test -race -count=1 -run TestRead_ReturnsAFlowThatChecksAndEmits
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q445, Q449)
func TestRead_ReturnsAFlowThatChecksAndEmits(t *testing.T) {
	for _, text := range []string{
		READ_FLOW,
		`{"text": "pass", "init": null}`,
		`{"constants": {"K": {"value": {"null": null}}}, "text": "pass"}`,
		`{"text": "print(nothing_declares_this)"}`,
	} {
		flow, err := runtime.Read([]byte(text))
		if err != nil {
			t.Fatalf(
				"%s: %v",
				text,
				err,
			)
		}

		err = runtime.Check(flow)
		if err != nil {
			t.Fatalf(
				"Check refuses %s: %v",
				text,
				err,
			)
		}

		script, err := runtime.Emit(flow)
		if err != nil {
			t.Fatalf(
				"Emit refuses %s: %v",
				text,
				err,
			)
		}

		_, err = runtime.Compile(&common.Source{
			Entry: "read.star",
			Text:  script,
		})
		if (err != nil) != strings.Contains(
			text,
			"nothing_declares_this",
		) {
			t.Fatalf(
				"Compile of the script of %s: %v",
				text,
				err,
			)
		}
	}
}

// BEGIN GOLDEN: no test writes these tables; edit by hand only to review.

// DECODER lists a flow the decoder refuses and the words Read gives. The words are the
// ones `lark run` printed at 5d51bae, less "<file>: " in front.
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q4, Q433, Q444)
//
//nolint:lll // a golden row
var (
	DECODER = []struct{ name, flow, want string }{
		{
			`a string where an object stands`,
			`{"main": {"statement": [{"call": {"function": "f", "args": ["west"]}}]}}`,
			`proto: syntax error (line 1:61): unexpected token "west" ` + TAGGED,
		},
		{
			`a number where an object stands`,
			`{"constants": {"X": {"value": 3}}, "text": "pass"}`,
			`proto: syntax error (line 1:31): unexpected token 3 ` + TAGGED,
		},
		{
			`a null in a list of values`,
			`{"constants": {"K": {"value": {"list": {"items": [null]}}}}, "text": "pass"}`,
			`proto: syntax error (line 1:51): unexpected token null ` + TAGGED,
		},
		{
			`empty text`,
			``,
			`proto: syntax error (line 1:1): unexpected token `,
		},
		{
			`blank text`,
			" \n\t ",
			`proto: syntax error (line 2:3): unexpected token `,
		},
		{
			`text that is no JSON`,
			"not json\n",
			`proto: syntax error (line 1:1): invalid value not`,
		},
		{
			`a field the schema does not know`,
			`{"nothing": 1}`,
			`proto: (line 1:2): unknown field "nothing"`,
		},
		{
			`an int past int64`,
			`{"constants": {"X": {"value": {"int": "9223372036854775808"}}}, "text": "pass"}`,
			`proto: (line 1:39): invalid value for int64 field int: "9223372036854775808"`,
		},
		{
			`an int that is a fraction`,
			`{"constants": {"X": {"value": {"int": "1.5"}}}, "text": "pass"}`,
			`proto: (line 1:39): invalid value for int64 field int: "1.5"`,
		},
		{
			`a string of 5,000 bytes where an object stands`,
			`{"constants": {"X": {"value": "` + strings.Repeat(
				"a",
				5000,
			) + `"}}, "text": "pass"}`,
			`proto: syntax error (line 1:31): unexpected token "<a>... (N bytes) ` + TAGGED,
		},
	}
)

// CHECKED lists a flow that decodes and that Check refuses, the words of Check, and
// the sentinel those words match.
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q445)
//
//nolint:lll // a golden row
var (
	CHECKED = []struct {
		name, flow, want string
		is               error
	}{
		{
			`a name declared twice`,
			`{"functions":[{"name":"f","body":"return 1"},{"name":"f","body":"return 2"}],` +
				`"main":{"statement":[{"call":{"function":"f"}}]}}`,
			`"f": a name is declared twice`,
			runtime.ErrDuplicate,
		},
		{
			`a value that has no kind`,
			`{"constants": {"K": {"value": {}}}, "text": "pass"}`,
			`"K": value has no kind (<nil>): no form`,
			runtime.ErrForm,
		},
	}
)

// NULLS lists a flow with a null the decoder would read as no value, and a word the
// refusal holds.
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q439, Q442)
//
//nolint:lll // a golden row
var (
	NULLS = []struct{ name, flow, want string }{
		{
			`the default of an argument`,
			`{"args": {"a": {"name": "a", "default": null}}, "text": "pass"}`,
			`flow.args{}.default`,
		},
		{
			`the value of a constant`,
			`{"constants": {"K": {"value": null}}, "text": "pass"}`,
			`flow.constants{}.value`,
		},
		{
			`a literal operand`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], ` +
				`"main": {"statement": [{"call": {"function": "f", ` +
				`"operands": [{"literal": null}]}}]}}`,
			`.operands[0].literal`,
		},
		{
			`the literal of a keyword`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], ` +
				`"main": {"statement": [{"call": {"function": "f", ` +
				`"keywords": [{"name": "p", "value": {"literal": null}}]}}]}}`,
			`.keywords[0].value.literal`,
		},
		{
			`the count of a loop`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], ` +
				`"main": {"statement": [{"loop": {"times": {"literal": null}, ` +
				`"body": {"call": {"function": "f"}}}}]}}`,
			`.loop.times.literal`,
		},
		{
			`an argument of a call`,
			`{"functions": [{"name": "f", "params": ["p"], "body": "pass"}], ` +
				`"main": {"statement": [{"call": {"function": "f", "args": [null]}}]}}`,
			`unexpected token null`,
		},
		{
			`an item of a list`,
			`{"constants": {"K": {"value": {"list": {"items": [null]}}}}, "text": "pass"}`,
			`unexpected token null`,
		},
		{
			`an entry of a map`,
			`{"constants": {"K": {"value": {"map": {"entries": {"k": null}}}}}, "text": "pass"}`,
			`unexpected token null`,
		},
		{
			`a null kind that holds 5`,
			`{"constants": {"K": {"value": {"null": 5}}}, "text": "pass"}`,
			`flow.constants{}.value.null`,
		},
		{
			`a null kind that holds its own name`,
			`{"constants": {"K": {"value": {"null": "NULL_VALUE"}}}, "text": "pass"}`,
			`flow.constants{}.value.null`,
		},
		{
			`a null kind that holds 0`,
			`{"constants": {"K": {"value": {"null": 0}}}, "text": "pass"}`,
			`flow.constants{}.value.null`,
		},
		{
			`a null kind inside a list`,
			`{"constants": {"K": {"value": {"list": {"items": [{"null": 1}]}}}}, ` +
				`"text": "pass"}`,
			`.null`,
		},
	}
)

// END GOLDEN
