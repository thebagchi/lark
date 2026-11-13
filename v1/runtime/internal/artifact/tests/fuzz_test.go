package tests

import (
	"sync/atomic"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

const (
	TOUCH_NAME = "touch"
)

var (
	// TOUCHED counts the calls of touch, the one name _Touching supplies.
	//
	// Revisions:
	//   - 2026-11-04 21:06: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	TOUCHED atomic.Int64
)

// _Touching is a plugin whose one builtin counts its calls, so a test can tell
// whether any script code ran.
//
// Revisions:
//   - 2026-11-04 21:06: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Touching struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Touching) Name() string {
	return TOUCH_NAME
}

// Values supplies touch.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Touching) Values() starlark.StringDict {
	return starlark.StringDict{
		TOUCH_NAME: starlark.NewBuiltin(
			TOUCH_NAME,
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				TOUCHED.Add(1)

				return starlark.None, nil
			},
		),
	}
}

// FuzzLoad feeds Load any bytes. Load never panics and never runs the code: the
// counter of touch, which touch.star calls at module level, stays at zero. A
// bundle it accepts holds its entry, and holds every module its programs load,
// each mapped to a unit of the bundle, so dropping _Covered fails the
// cleared_loads seed. Save is not asked: a loaded artifact has no syntax tree,
// and Save refuses it with spelling.ErrMain. The seeds are the bundles Save writes for
// the fixtures, bundles that name plugins, the multi-unit and cleared-loads
// bundles, the crash input of B126, and bytes that are not a bundle.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-04 19:33: checks the entry, the load map and that no code runs (C89 fix round 1)
//   - 2026-11-05 16:54: seeds bundles that name plugins (C79)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-09 11:25: the three bundles of testdata/fuzz/FuzzLoad are seeded by f.Add (C114,
//     Q283)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrMain, formerly ErrNoMain (C121, Q348)
func FuzzLoad(f *testing.F) {
	plugins := artifact.WithPlugins(new(_Touching))

	for _, name := range []string{
		"app.star",
		"lib.star",
		"spread.star",
		"touch.star",
	} {
		built, err := artifact.Compile(
			&common.Source{
				Entry:  name,
				Text:   []byte(FIXTURES[name]),
				Loader: _Loader(),
			},
			plugins,
		)
		if err != nil {
			f.Fatalf(
				"compile %s: %v",
				name,
				err,
			)
		}

		saved, err := built.Save()
		if err != nil {
			continue
		}

		f.Add(saved)
	}

	// The plugins field: one name the load is given, one it is not, and one
	// with an empty name.
	for _, names := range [][]string{
		{TOUCH_NAME},
		{"absent"},
		{"", TOUCH_NAME, "absent"},
	} {
		raw, err := proto.Marshal(&artifactpb.Artifact{
			Entry:   "x",
			Plugins: names,
		})
		if err != nil {
			f.Fatalf(
				"marshal: %v",
				err,
			)
		}

		f.Add(raw)
	}

	// The bundles the corpus held: multi_unit, cleared_loads and the crash
	// input of B126, whose bytes CRASH_CORPUS holds.
	f.Add([]byte(
		"\n\bapp.star\x12s\n\blib.star\x12g!sky:\x00\x00\x00\x1c\x10" +
			"\x00\x00\x02\x04\x04\x02\f\x04\n\x14\x04\x02\x00\x10\x02\x00" +
			"\x00\x00\x00\x02\x00\x00\x00\x00\x02\f\x04\x02\x00\f\x04\xac" +
			"\x02\x88\x80\x02\x02\x02\x04\x18\x00\x00\x04\x02\x00\x00\x00" +
			"\x02lib.stardouble<toplevel>/\x001\x004\x00\x1a!double5\x00." +
			"\x00\r!x\x12\xdc\x01\n\vhelper.star\x12\xb6\x01!skyU\x00\x00" +
			"\x00\x1c\x16\x02\x10\x06\f\x00\x04\x00\f\x00\x10\x02\f\n\n" +
			"\x14\x06\x02\x00$\x06\x00\x80\x80\x02\x80\x84\x02\x02\f\x06&" +
			"\x02\x00\x00\x04\x00\x00\x00\x00\x02\f\n\x02\x00\x16\n\xac" +
			"\x02\x9c\x80\x01\xfc\x81\x01\x98\xc0\x01\xf8\x81\x01\x02\x02" +
			"\n\x18\x00\x02\f\x06&\x04\x02\x00\x00\x00\x02helper.starlib." +
			"stardoublelib.startriple<toplevel>.\x00.\x012\x019\x005\x00/" +
			"\x011\x004\x00\x1a!doubletriple7\x005\x00@\x80\x025\x00\v!xd" +
			"ouble\x1a\x14\n\blib.star\x12\blib.star\x12\xb6\x02\n\bapp.s" +
			"tar\x12\xf7\x01!skyg\x00\x00\x00\x1c\x10\x04\x10\x02\f\x16" +
			"\x04\f\x00\f\x00\f\x00\x10\x00\f\x00\x16\x04\x04\x04\x06\x02" +
			"\b\b\n\x14\x02\x02\x008\b\x00\x80\x80\x02\x80\x82\x04\x80" +
			"\x84\x02\x04\f\x02&\f\x04,\x04\x00\x02\x00\x04\x00\x00\x00" +
			"\x00\x02\b\b\x02\x00 \n\xac\x02\x98\x80\x02\x98\xc0\x01\x98" +
			"\x80\x02\xe0\xc1\x01\x00\x00\x04\f\x02&\f\x04,\x06\x00\x00" +
			"\x00\x00\x02app.starlib.starhelper.stardoublelib.startripleh" +
			"elper.starmain<toplevel>.\x00.\x012\x019\x00.\x02.\x032\x019" +
			"\x015\x005\x01/\x021\x004\x00\x1a!doubletriplemain7\x00.\x04" +
			"@\x80\x027\x01.\x05@\x80\x02\v!doubletriple\x1a\x14\n\blib.s" +
			"tar\x12\blib.star\x1a\x1a\n\vhelper.star\x12\vhelper.star",
	))
	f.Add([]byte(
		"\n\bapp.star\x12s\n\blib.star\x12g!sky:\x00\x00\x00\x1c\x10" +
			"\x00\x00\x02\x04\x04\x02\f\x04\n\x14\x04\x02\x00\x10\x02\x00" +
			"\x00\x00\x00\x02\x00\x00\x00\x00\x02\f\x04\x02\x00\f\x04\xac" +
			"\x02\x88\x80\x02\x02\x02\x04\x18\x00\x00\x04\x02\x00\x00\x00" +
			"\x02lib.stardouble<toplevel>/\x001\x004\x00\x1a!double5\x00." +
			"\x00\r!x\x12\xc6\x01\n\vhelper.star\x12\xb6\x01!skyU\x00\x00" +
			"\x00\x1c\x16\x02\x10\x06\f\x00\x04\x00\f\x00\x10\x02\f\n\n" +
			"\x14\x06\x02\x00$\x06\x00\x80\x80\x02\x80\x84\x02\x02\f\x06&" +
			"\x02\x00\x00\x04\x00\x00\x00\x00\x02\f\n\x02\x00\x16\n\xac" +
			"\x02\x9c\x80\x01\xfc\x81\x01\x98\xc0\x01\xf8\x81\x01\x02\x02" +
			"\n\x18\x00\x02\f\x06&\x04\x02\x00\x00\x00\x02helper.starlib." +
			"stardoublelib.startriple<toplevel>.\x00.\x012\x019\x005\x00/" +
			"\x011\x004\x00\x1a!doubletriple7\x005\x00@\x80\x025\x00\v!xd" +
			"ouble\x12\x84\x02\n\bapp.star\x12\xf7\x01!skyg\x00\x00\x00" +
			"\x1c\x10\x04\x10\x02\f\x16\x04\f\x00\f\x00\f\x00\x10\x00\f" +
			"\x00\x16\x04\x04\x04\x06\x02\b\b\n\x14\x02\x02\x008\b\x00" +
			"\x80\x80\x02\x80\x82\x04\x80\x84\x02\x04\f\x02&\f\x04,\x04" +
			"\x00\x02\x00\x04\x00\x00\x00\x00\x02\b\b\x02\x00 \n\xac\x02" +
			"\x98\x80\x02\x98\xc0\x01\x98\x80\x02\xe0\xc1\x01\x00\x00\x04" +
			"\f\x02&\f\x04,\x06\x00\x00\x00\x00\x02app.starlib.starhelper" +
			".stardoublelib.startriplehelper.starmain<toplevel>.\x00.\x01" +
			"2\x019\x00.\x02.\x032\x019\x015\x005\x01/\x021\x004\x00\x1a!" +
			"doubletriplemain7\x00.\x04@\x80\x027\x01.\x05@\x80\x02\v!dou" +
			"bletriple",
	))
	f.Add([]byte(CRASH_CORPUS))

	for _, seed := range [][]byte{
		nil,
		[]byte("not a bundle"),
		{0x0a, 0x01, 'x'},
		{0xff, 0xff, 0xff},
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		bundle []byte,
	) {
		var (
			before = TOUCHED.Load()
			_, err = artifact.Load(
				bundle,
				plugins,
			)
		)

		if TOUCHED.Load() != before {
			t.Fatalf(
				"Load ran script code on %x",
				bundle,
			)
		}

		if err != nil {
			return
		}

		_Consistent(
			t,
			bundle,
			bundle,
		)
	})
}

// _Consistent ends the test unless saved, the bytes Load accepted, holds its
// entry, and every module its programs load is mapped to a unit of the bundle.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Consistent(
	t *testing.T,
	from []byte,
	saved []byte,
) {
	t.Helper()

	var bundle = new(artifactpb.Artifact)
	if err := proto.Unmarshal(
		saved,
		bundle,
	); err != nil {
		t.Fatalf(
			"accepted %x is not a bundle: %v",
			from,
			err,
		)
	}

	names := map[string]bool{}
	for _, unit := range bundle.GetUnits() {
		names[unit.GetName()] = true
	}

	if !names[bundle.GetEntry()] {
		t.Fatalf(
			"Load accepted %x, but its entry %q is no unit",
			from,
			bundle.GetEntry(),
		)
	}

	for _, unit := range bundle.GetUnits() {
		program, err := _Program(unit.GetCode())
		if err != nil {
			t.Fatalf(
				"Load accepted %x, but unit %q is no program: %v",
				from,
				unit.GetName(),
				err,
			)
		}

		for idx := range program.NumLoads() {
			spelling, _ := program.Load(idx)

			if !names[unit.GetLoads()[spelling]] {
				t.Fatalf(
					"Load accepted %x, but unit %q loads %q, unmapped",
					from,
					unit.GetName(),
					spelling,
				)
			}
		}
	}
}
