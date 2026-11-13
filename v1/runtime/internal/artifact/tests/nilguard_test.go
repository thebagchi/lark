package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
)

// TestNilGuards proves each public entry of this package refuses a nil source
// with script.ErrSource, and skips a nil option or plugin, without a panic.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-07 23:04: uses the options and artifact types moved to common (C91w6)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource; uses Merge, formerly Amalgamate
//     (C121, Q348, Q350)
func TestNilGuards(t *testing.T) {
	var (
		ok = &common.Source{
			Entry: "a.star",
			Text:  []byte("def main():\n    return 1\n"),
		}
		cases = []struct {
			name    string
			call    func() error
			wantErr error
		}{
			{
				name: "Compile nil source",
				call: func() error {
					_, err := artifact.Compile(nil)

					return err
				},
				wantErr: script.ErrSource,
			},
			{
				name: "Merge nil source",
				call: func() error {
					_, err := artifact.Merge(nil)

					return err
				},
				wantErr: script.ErrSource,
			},
			{
				name: "Compile nil option and nil plugin",
				call: func() error {
					_, err := artifact.Compile(
						ok,
						nil,
						artifact.WithPlugins(nil),
					)

					return err
				},
			},
			{
				name: "Apply nil run option",
				call: func() error {
					given := artifact.Apply([]common.RunOption{nil})
					if given == nil {
						return errors.New("Apply returned nil")
					}

					return nil
				},
			},
		}
	)

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := tc.call()
				if !errors.Is(
					err,
					tc.wantErr,
				) {
					t.Fatalf(
						"want %v, got %v",
						tc.wantErr,
						err,
					)
				}
			},
		)
	}
}

// _PanickingValues is a host plugin whose Values panics.
//
// Revisions:
//   - 2026-11-05 19:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _PanickingValues struct{}

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (B123, C96)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (*_PanickingValues) Name() string {
	return "panicking"
}

// Values panics.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (B123, C96)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (*_PanickingValues) Values() starlark.StringDict {
	panic("values failed")
}

// TestCompile_PluginValuesPanics proves B123: Compile returns an error that
// names the plugin, and does not panic, when a plugin's Values panics.
//
// Revisions:
//   - 2026-11-05 16:55: initial creation (B123, C96)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_PluginValuesPanics(t *testing.T) {
	var (
		source = &common.Source{
			Entry: "a.star",
			Text:  []byte("def main():\n    return 1\n"),
		}
		_, err = artifact.Compile(
			source,
			artifact.WithPlugins(&_PanickingValues{}),
		)
	)
	if err == nil {
		t.Fatal("err is nil, want the panic as an error")
	}

	if !strings.Contains(
		err.Error(),
		"panicking",
	) {
		t.Fatalf(
			"err = %q, want the plugin name",
			err,
		)
	}
}

// _Fickle is a host plugin that checks every tree. Its Values panics from the
// second call on, or its Check panics, as the test asks. Both panic with an
// error value, so the chain can be read.
//
// Revisions:
//   - 2026-11-05 19:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Fickle struct {
	calls      int
	valuesFrom int
	checkPanic bool
}

// ErrFickle is what _Fickle panics with.
//
// Revisions:
//   - 2026-11-05 19:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	ErrFickle = errors.New("fickle")
)

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation (B123, C96 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (*_Fickle) Name() string {
	return "fickle"
}

// Values supplies one name, and panics from the call numbered valuesFrom on.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation (B123, C96 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Fickle) Values() starlark.StringDict {
	p.calls++

	if p.valuesFrom > 0 && p.calls >= p.valuesFrom {
		panic(ErrFickle)
	}

	return starlark.StringDict{"fickle_name": starlark.None}
}

// Check panics when asked to, and otherwise accepts the tree.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation (B123, C96 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Fickle) Check(*syntax.File) error {
	if p.checkPanic {
		panic(ErrFickle)
	}

	return nil
}

// TestCompile_HostPluginPanicsLater proves every call from Compile into a host
// plugin is guarded: a Values that panics on its second call (the shadow
// check) and a Check that panics each give an error that names the plugin and
// keeps the panic value in the chain, and Compile does not panic.
//
// Revisions:
//   - 2026-11-05 17:40: initial creation (B123, C96 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompile_HostPluginPanicsLater(t *testing.T) {
	for _, tc := range []struct {
		name   string
		plugin *_Fickle
	}{
		{
			"values on the second call",
			&_Fickle{
				valuesFrom: 2,
			},
		},
		{
			"check",
			&_Fickle{
				checkPanic: true,
			},
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					source = &common.Source{
						Entry: "a.star",
						Text:  []byte("def main():\n    return 1\n"),
					}
					_, err = artifact.Compile(
						source,
						artifact.WithPlugins(tc.plugin),
					)
				)
				if err == nil {
					t.Fatal("err is nil, want the panic as an error")
				}

				if !strings.Contains(
					err.Error(),
					"fickle",
				) {
					t.Fatalf(
						"err = %q, want the plugin name",
						err,
					)
				}

				if !errors.Is(
					err,
					ErrFickle,
				) {
					t.Fatalf(
						"err = %q, want the panic value in the chain",
						err,
					)
				}
			},
		)
	}
}

// _Odd is a host plugin with no Check. Its Name panics when unnamed is set,
// and its Values panics from the call numbered valuesFrom on. Both panic with
// ErrFickle.
//
// Revisions:
//   - 2026-11-05 19:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Odd struct {
	calls      int
	valuesFrom int
	unnamed    bool
}

// Name is the plugin's name, or a panic.
//
// Revisions:
//   - 2026-11-05 18:40: initial creation (B123, C96 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Odd) Name() string {
	if p.unnamed {
		panic(ErrFickle)
	}

	return "plain"
}

// Values supplies one name, and panics from the call numbered valuesFrom on.
//
// Revisions:
//   - 2026-11-05 18:40: initial creation (B123, C96 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Odd) Values() starlark.StringDict {
	p.calls++

	if p.valuesFrom > 0 && p.calls >= p.valuesFrom {
		panic(ErrFickle)
	}

	return starlark.StringDict{"plain_name": starlark.None}
}

// TestCompileAndLoad_HostPluginPanicsInNeeds proves the calls that record and
// check which plugins a bundle needs (_Owners and _Supplied) are guarded: a
// Values that panics on its second call, and a Name that panics, each give
// Compile an error, and a Name that panics gives Load an error. The panic value
// stays in the chain.
//
// Revisions:
//   - 2026-11-05 18:40: initial creation (B123, C96 fix round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCompileAndLoad_HostPluginPanicsInNeeds(t *testing.T) {
	var source = &common.Source{
		Entry: "a.star",
		Text:  []byte("def main():\n    return 1\n"),
	}

	for _, tc := range []struct {
		name   string
		plugin *_Odd
		load   bool
	}{
		{
			"compile values on the second call",
			&_Odd{
				valuesFrom: 2,
			},
			false,
		},
		{
			"compile name",
			&_Odd{
				unnamed: true,
			},
			false,
		},
		{
			"load name",
			&_Odd{
				unnamed: true,
			},
			true,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				if tc.load {
					built, cerr := artifact.Compile(source)
					if cerr != nil {
						t.Fatal(cerr)
					}

					bundle, serr := built.Save()
					if serr != nil {
						t.Fatal(serr)
					}

					_, err = artifact.Load(
						bundle,
						artifact.WithPlugins(tc.plugin),
					)
				} else {
					_, err = artifact.Compile(
						source,
						artifact.WithPlugins(tc.plugin),
					)
				}

				if !errors.Is(
					err,
					ErrFickle,
				) {
					t.Fatalf(
						"err = %v, want the panic value in the chain",
						err,
					)
				}
			},
		)
	}
}
