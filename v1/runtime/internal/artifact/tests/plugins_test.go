package tests

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

var (
	// OLD_BUNDLE is the bundle Save wrote at the base of C79, from touch.star with
	// the touch plugin given, before the plugins field existed.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/old_touch.bundle
	//     (C114, Q283)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	OLD_BUNDLE = []byte(
		"\x0a\x0atouch.star\x12\xe6\x01\x0a\x0atouch.star\x12k!sky:" +
			"\x00\x00\x00\x1c\x14\x00\x02\x0a\x02\x04\x02\x02\x08\x08\x0a" +
			"\x14\x04\x02\x00\x1a\x06\x00\x94\x80\x01\xec\xc5\x01\x00\x00" +
			"\x00\x02\x00\x00\x00\x00\x02\x08\x08\x02\x00\x06\x02\x00\x00" +
			"\x00\x00\x02\x00\x00\x00\x00\x02touch.startouchmain<toplevel" +
			">;\x00@\x00\x03/\x001\x004\x00\x1a!main.\x00!\"k# Calls touc" +
			"h at module level, so a run would count and a load must not." +
			"\x0atouch()\x0a\x0adef main():\x0a    return 1\x0a",
	)
)

// _Saved compiles name with the host plugin touch given and returns the bundle
// Save writes, or ends the test.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Saved(
	t *testing.T,
	name string,
) []byte {
	t.Helper()

	built, err := artifact.Compile(
		&common.Source{
			Entry: name,
			Text: _Fixture(
				t,
				name,
			),
			Loader: _Loader(),
		},
		artifact.WithPlugins(new(_Touching)),
	)
	if err != nil {
		t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save %s: %v",
			name,
			err,
		)
	}

	return saved
}

// _Named is the plugin names a bundle carries, or ends the test.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Named(
	t *testing.T,
	bundle []byte,
) []string {
	t.Helper()

	var read = new(artifactpb.Artifact)

	err := proto.Unmarshal(
		bundle,
		read,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	return read.GetPlugins()
}

// TestLoad_NeedsItsHostPlugin proves a bundle made with a host plugin is
// refused, with ErrPlugin naming the plugin, when it is loaded without
// that plugin, and loads when the plugin is given.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79, B111)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrPlugin, formerly ErrPluginMissing (C121, Q348)
func TestLoad_NeedsItsHostPlugin(t *testing.T) {
	saved := _Saved(
		t,
		"touch.star",
	)

	t.Run(
		"without the plugin",
		func(t *testing.T) {
			_, err := artifact.Load(saved)
			if !errors.Is(
				err,
				artifact.ErrPlugin,
			) {
				t.Fatalf(
					"got %v, want ErrPlugin",
					err,
				)
			}

			if !strings.Contains(
				err.Error(),
				`"`+TOUCH_NAME+`"`,
			) {
				t.Fatalf(
					"the text %q does not name the plugin, quoted",
					err,
				)
			}
		},
	)

	t.Run(
		"with the plugin",
		func(t *testing.T) {
			_, err := artifact.Load(
				saved,
				artifact.WithPlugins(new(_Touching)),
			)
			if err != nil {
				t.Fatalf(
					"load: %v",
					err,
				)
			}
		},
	)

	t.Run(
		"the plugin is named in the bundle",
		func(t *testing.T) {
			if !slices.Contains(
				_Named(
					t,
					saved,
				),
				TOUCH_NAME,
			) {
				t.Fatalf(
					"plugins %v lack %q",
					_Named(
						t,
						saved,
					),
					TOUCH_NAME,
				)
			}
		},
	)
}

// TestLoad_ASpareOrAbsentPluginIsFine proves only the plugins a script uses are
// saved: a plugin given at compile and not used is not needed at load, and a
// script of the runtime's own names needs nothing the host adds.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_ASpareOrAbsentPluginIsFine(t *testing.T) {
	saved := _Saved(
		t,
		"app.star",
	)

	if slices.Contains(
		_Named(
			t,
			saved,
		),
		TOUCH_NAME,
	) {
		t.Fatalf(
			"plugins %v name a plugin the script does not use",
			_Named(
				t,
				saved,
			),
		)
	}

	_, err := artifact.Load(saved)
	if err != nil {
		t.Fatalf(
			"load with no host plugin: %v",
			err,
		)
	}

	built, err := artifact.Compile(&common.Source{
		Entry: "app.star",
		Text: _Fixture(
			t,
			"app.star",
		),
		Loader: _Loader(),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	plain, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	_, err = artifact.Load(plain)
	if err != nil {
		t.Fatalf(
			"a builtin-only script needs nothing: %v",
			err,
		)
	}
}

// TestLoad_AnOldBundleHasNoCheck proves a bundle written before the field
// existed loads with no check, even though its script calls a host plugin. The
// file was made at the base of C79 by Save, from touch.star with the touch
// plugin given.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLoad_AnOldBundleHasNoCheck(t *testing.T) {
	if names := _Named(
		t,
		OLD_BUNDLE,
	); len(names) != 0 {
		t.Fatalf(
			"the old bundle names plugins %v",
			names,
		)
	}

	_, err := artifact.Load(OLD_BUNDLE)
	if err != nil {
		t.Fatalf(
			"an old bundle should load: %v",
			err,
		)
	}
}

// TestLoad_BoundsAPluginName proves the name of a missing plugin is quoted and
// bounded in the text, whatever length the file gives it.
//
// Revisions:
//   - 2026-11-05 16:54: initial creation (C79, Q4)
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrPlugin, formerly ErrPluginMissing (C121, Q348)
func TestLoad_BoundsAPluginName(t *testing.T) {
	const LONG = 1 << 20

	for _, test := range []struct {
		name string
		with string
		fits string
	}{
		{
			name: "control bytes are escaped",
			with: "bad\x00name\n",
			fits: `"bad\x00name\n"`,
		},
		{
			name: "a long name is cut",
			with: strings.Repeat(
				"p",
				LONG,
			),
			fits: `bytes)`,
		},
	} {
		t.Run(
			test.name,
			func(t *testing.T) {
				bundle := _Handmade(
					t,
					"app.star",
				)
				bundle.Plugins = []string{test.with}

				raw, err := proto.Marshal(bundle)
				if err != nil {
					t.Fatalf(
						"marshal: %v",
						err,
					)
				}

				_, err = artifact.Load(raw)
				if !errors.Is(
					err,
					artifact.ErrPlugin,
				) {
					t.Fatalf(
						"got %v, want ErrPlugin",
						err,
					)
				}

				if len(err.Error()) > 1000 {
					t.Fatalf(
						"the text is %d bytes",
						len(err.Error()),
					)
				}

				if !strings.Contains(
					err.Error(),
					test.fits,
				) {
					t.Fatalf(
						"the text %q lacks %q",
						err,
						test.fits,
					)
				}
			},
		)
	}
}
