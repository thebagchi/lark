package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// _Nilled is a plugin that supplies a nil value, so building its environment
// fails.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
type _Nilled struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
func (n *_Nilled) Name() string {
	return "nilled"
}

// Values supplies one nil name.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
func (n *_Nilled) Values() starlark.StringDict {
	return starlark.StringDict{"x": nil}
}

// _Cut proves err holds the first QUOTE_LIMIT bytes of name and the length
// marker, and is short.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
func _Cut(
	t *testing.T,
	err error,
	prefix string,
	name string,
) {
	t.Helper()

	if err == nil {
		t.Fatal("no error")
	}

	text := err.Error()

	if len(text) > 1000 {
		t.Fatalf(
			"error is %d bytes, want a bound",
			len(text),
		)
	}

	want := prefix + name[:common.QUOTE_LIMIT] + "... ("
	if !strings.HasPrefix(
		text,
		want,
	) {
		t.Fatalf(
			"error %.200q lacks the cut name after %q",
			text,
			prefix,
		)
	}
}

// _Bundled is the bundle of a script that compiles, changed by change.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
func _Bundled(
	t *testing.T,
	change func(*artifactpb.Artifact),
) []byte {
	t.Helper()

	built, err := artifact.Compile(&common.Source{
		Entry: WALK_ENTRY,
		Text:  []byte(WALK_MAIN),
	})
	if err != nil {
		t.Fatal(err)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatal(err)
	}

	message := new(artifactpb.Artifact)

	err = proto.Unmarshal(
		saved,
		message,
	)
	if err != nil {
		t.Fatal(err)
	}

	change(message)

	changed, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	return changed
}

// TestNames_EveryErrorPrefixIsBounded proves a name of 100,000 bytes does not
// fill the error that starts with the entry or a unit name: compile, merge,
// load, entry, save without a main, and a unit that loads what the bundle does
// not hold (Q4, B239). The encode prefix cannot be reached: Program.Write goes
// to a bytes.Buffer and does not fail.
//
// Revisions:
//   - 2026-11-10 16:06: initial creation (C117, B239)
//   - 2026-11-10 22:40: uses Merge, formerly Amalgamate (C121, Q350)
func TestNames_EveryErrorPrefixIsBounded(t *testing.T) {
	var (
		long = strings.Repeat(
			"e",
			100000,
		)
		failed = artifact.WithPlugins(new(_Nilled))
	)

	t.Run(
		"compile",
		func(t *testing.T) {
			_, err := artifact.Compile(
				&common.Source{
					Entry: long,
					Text:  []byte(WALK_MAIN),
				},
				failed,
			)
			_Cut(
				t,
				err,
				"compile ",
				long,
			)
		},
	)

	t.Run(
		"merge",
		func(t *testing.T) {
			_, err := artifact.Merge(
				&common.Source{
					Entry: long,
					Text:  []byte(WALK_MAIN),
				},
				failed,
			)
			_Cut(
				t,
				err,
				"merge ",
				long,
			)
		},
	)

	t.Run(
		"load",
		func(t *testing.T) {
			_, err := artifact.Load(
				_Bundled(
					t,
					func(message *artifactpb.Artifact) {
						message.Entry = long
						message.Units[0].Name = long
					},
				),
				failed,
			)
			_Cut(
				t,
				err,
				"load ",
				long,
			)
		},
	)

	t.Run(
		"entry",
		func(t *testing.T) {
			_, err := artifact.Load(_Bundled(
				t,
				func(message *artifactpb.Artifact) {
					message.Entry = long
				},
			))
			_Cut(
				t,
				err,
				"entry ",
				long,
			)
		},
	)

	t.Run(
		"save without a main",
		func(t *testing.T) {
			built, err := artifact.Compile(&common.Source{
				Entry: long,
				Text:  []byte("x = 1\n"),
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = built.Save()
			_Cut(
				t,
				err,
				"",
				long,
			)
		},
	)

	t.Run(
		"unit loads what the bundle does not hold",
		func(t *testing.T) {
			_, err := artifact.Load(_Bundled(
				t,
				func(message *artifactpb.Artifact) {
					message.Entry = long
					message.Units[0].Name = long
					message.Units[0].Loads = map[string]string{
						"a.star": "missing.star",
					}
				},
			))
			_Cut(
				t,
				err,
				"",
				long,
			)
		},
	)

	t.Run(
		"entry that takes a parameter",
		func(t *testing.T) {
			built, err := artifact.Compile(&common.Source{
				Entry: long,
				Text:  []byte("def main(a):\n    return a\n"),
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = built.Save()
			if err == nil || len(err.Error()) > 1000 {
				t.Fatalf(
					"got %.200v, want a bounded error",
					err,
				)
			}

			if strings.Count(
				err.Error(),
				"... (",
			) != 2 {
				t.Fatalf(
					"got %.300q, want the entry and the position cut",
					err,
				)
			}
		},
	)

	t.Run(
		"call that does not fit",
		func(t *testing.T) {
			_, err := artifact.Compile(&common.Source{
				Entry: long,
				Text: []byte(
					"def f(a):\n    return a\n" +
						"def main():\n    return f()\n",
				),
			})
			_Cut(
				t,
				err,
				"",
				long,
			)
		},
	)

	t.Run(
		"initialise",
		func(t *testing.T) {
			built, err := artifact.Compile(&common.Source{
				Entry: long,
				Text:  []byte("x = fail(\"boom\")\ndef main():\n    return x\n"),
			})
			if err != nil {
				t.Fatal(err)
			}

			_, err = artifact.Run(
				t.Context(),
				built,
			)
			_Cut(
				t,
				err,
				"initialise ",
				long,
			)
		},
	)
}
