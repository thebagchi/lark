package tests

import (
	"errors"
	"path"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// TestRun_ABytesConstantRunsAsBytesFromTheFlow proves the flow of a script that
// holds a bytes constant, or an argument with a bytes default, runs to the value
// the script runs to, and the value is bytes: the flow holds a value of the bytes
// kind, where it once held a string (B290, Q407). A dict with a key that is no
// string is the case that is still init text (Q406, Q423). An argument given at
// run time wins over the default in both.
//
// The check: go test -race -count=1 -run TestRun_ABytesConstantRunsAsBytesFromTheFlow
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 00:45: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: the flow holds a bytes value, not init text (C132, Q406, Q407)
func TestRun_ABytesConstantRunsAsBytesFromTheFlow(t *testing.T) {
	var tests = []struct {
		name string
		text string
		args map[string]any
		want string
		kind string
	}{
		{
			"a list of bytes",
			"X = [b\"0\"]\ndef main():\n    return X\n",
			nil,
			"[b\"0\"]",
			"list",
		},
		{
			"bytes",
			"X = b\"\\x00ab\"\ndef main():\n    return X\n",
			nil,
			"b\"\\x00ab\"",
			"bytes",
		},
		{
			"the element of a list of bytes",
			"X = [[1, b\"deep\"]]\ndef main():\n    return X[0][1]\n",
			nil,
			"b\"deep\"",
			"bytes",
		},
		{
			"a dict with a bytes value",
			"X = {\"k\": b\"v\"}\ndef main():\n    return X[\"k\"]\n",
			nil,
			"b\"v\"",
			"bytes",
		},
		{
			"a bytes constant read by a function",
			"X = b\"f\"\ndef f():\n    return X\ndef main():\n    return f()\n",
			nil,
			"b\"f\"",
			"bytes",
		},
		{
			"a constant that is a call with a bytes argument",
			"def f(a):\n    return a\nX = f(b\"x\")\ndef main():\n    return X\n",
			nil,
			"b\"x\"",
			"bytes",
		},
		{
			"a bytes dict key",
			"X = {b\"k\": 1}\ndef main():\n    return X\n",
			nil,
			"{b\"k\": 1}",
			"dict",
		},
		{
			"an argument with a bytes default",
			"a = arg(\"a\", b\"dflt\")\ndef main():\n    return a\n",
			nil,
			"b\"dflt\"",
			"bytes",
		},
		{
			"an argument with a bytes default, given a string",
			"a = arg(\"a\", b\"dflt\")\ndef main():\n    return a\n",
			map[string]any{"a": "given"},
			"\"given\"",
			"string",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var source = &common.Source{
					Entry: "bytes.star",
					Text:  []byte(tc.text),
				}

				want, err := runtime.Run(
					t.Context(),
					source,
					runtime.WithArgs(tc.args),
				)
				if err != nil {
					t.Fatalf(
						"run the script: %v",
						err,
					)
				}

				if want.String() != tc.want || want.Type() != tc.kind {
					t.Fatalf(
						"the script gives %v (%s), want %v (%s)",
						want,
						want.Type(),
						tc.want,
						tc.kind,
					)
				}

				flow, err := runtime.Derive(source)
				if err != nil {
					t.Fatal(err)
				}

				sent, err := protojson.Marshal(flow)
				if err != nil {
					t.Fatal(err)
				}

				var read = new(workflowpb.Flow)

				err = protojson.Unmarshal(
					sent,
					read,
				)
				if err != nil {
					t.Fatalf(
						"read the flow back from %s: %v",
						sent,
						err,
					)
				}

				text, err := runtime.Emit(read)
				if err != nil {
					t.Fatal(err)
				}

				got, err := runtime.Run(
					t.Context(),
					&common.Source{
						Entry: "bytes_flow.star",
						Text:  text,
					},
					runtime.WithArgs(tc.args),
				)
				if err != nil {
					t.Fatalf(
						"run the flow: %v",
						err,
					)
				}

				if got.String() != want.String() || got.Type() != want.Type() {
					t.Fatalf(
						"the flow gives %v (%s), the script %v (%s)",
						got,
						got.Type(),
						want,
						want.Type(),
					)
				}
			},
		)
	}
}

// _Bytes is a Loader that serves one module, which holds a constant that is a
// dict with a key that is no string.
//
// Revisions:
//   - 2026-11-13 01:19: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: the module holds a dict with a bytes key, not a list of bytes, which
//     a flow value holds now (C132, Q406, Q407)
type _Bytes struct{}

// Resolve reads target as a name beside from.
//
// Revisions:
//   - 2026-11-13 01:19: initial creation (C130, B290, Q406)
func (m *_Bytes) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the module.
//
// Revisions:
//   - 2026-11-13 01:19: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: the module holds a dict with a bytes key (C132, Q406, Q407)
func (m *_Bytes) Load(string) ([]byte, error) {
	return []byte("X = {b\"k\": 0}\ndef f():\n    return X\n"), nil
}

// TestDerive_RefusesATextConstantOfALoadedModule proves a constant of a module the
// script loads that is a dict with a bytes key is not carried: a flow has one
// init, and a module's statement has no place in it, so it is refused as any
// constant of a module a flow cannot hold is, with the words of that refusal,
// while the script runs (Q406). A list of bytes in a module is a value, and is
// carried.
//
// The check: go test -race -count=1 -run TestDerive_RefusesATextConstantOfALoadedModule
// ./v1/runtime/tests
//
// Revisions:
//   - 2026-11-13 01:19: initial creation (C130, B290, Q406)
//   - 2026-11-13 10:15: renamed from TestDerive_RefusesABytesConstantOfALoadedModule, and the
//     module holds a dict with a bytes key (C132, Q406, Q407)
func TestDerive_RefusesATextConstantOfALoadedModule(t *testing.T) {
	var source = &common.Source{
		Entry: "main.star",
		Text: []byte(
			"load(\"module.star\", \"f\")\ndef main():\n    return f()\n",
		),
		Loader: &_Bytes{},
	}

	got, err := runtime.Run(
		t.Context(),
		source,
	)
	if err != nil || got.String() != "{b\"k\": 0}" {
		t.Fatalf(
			"run the script: %v, %v",
			got,
			err,
		)
	}

	_, err = runtime.Derive(source)
	if !errors.Is(
		err,
		runtime.ErrCarry,
	) || !strings.Contains(
		err.Error(),
		"constant cannot be carried",
	) {
		t.Fatalf(
			"derive: err = %v, want the refusal of a constant",
			err,
		)
	}
}
