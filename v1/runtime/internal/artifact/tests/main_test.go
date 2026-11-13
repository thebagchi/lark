package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/core"
)

const (
	// APP_FIXTURE is the app fixture: an entry that loads lib.star and helper.star and returns
	// double(2) + triple(3), which is 13.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/app.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	APP_FIXTURE = `
load("lib.star", "double")
load("helper.star", "triple")

def main():
    return double(2) + triple(3)
`

	// LIB_FIXTURE is a module that defines double and no entry point, because nothing starts a
	// run in it.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/lib.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LIB_FIXTURE = `
# A module. It defines no entry point, because nothing starts a run here.
def double(x):
    return x * 2
`

	// HELPER_FIXTURE is a module that loads lib.star, as APP_FIXTURE does, so one compile
	// reaches lib.star by two routes.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/helper.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HELPER_FIXTURE = `
# Loads the same module the entry script does, so one compile reaches lib.star
# by two routes.
load("lib.star", "double")

def triple(x):
    return double(x) + x
`

	// MAINARGS_FIXTURE is a script whose main takes a required parameter, so it has no entry
	// point.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/mainargs.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	MAINARGS_FIXTURE = `
def main(x):
    return x
`

	// TOUCH_FIXTURE is a script that calls touch at module level, so a run would count and a
	// load must not.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/touch.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	TOUCH_FIXTURE = `
# Calls touch at module level, so a run would count and a load must not.
touch()

def main():
    return 1
`

	// SPREAD_FIXTURE is an entry that loads six modules and adds what their functions return.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_FIXTURE = `
load("spread_1.star", "f1")
load("spread_2.star", "f2")
load("spread_3.star", "f3")
load("spread_4.star", "f4")
load("spread_5.star", "f5")
load("spread_6.star", "f6")

def main():
    return f1() + f2() + f3() + f4() + f5() + f6()
`

	// SPREAD_1 is the module spread_1.star, which defines f1 returning 1.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_1.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_1 = `
def f1():
    return 1
`

	// SPREAD_2 is the module spread_2.star, which defines f2 returning 2.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_2.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_2 = `
def f2():
    return 2
`

	// SPREAD_3 is the module spread_3.star, which defines f3 returning 3.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_3.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_3 = `
def f3():
    return 3
`

	// SPREAD_4 is the module spread_4.star, which defines f4 returning 4.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_4.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_4 = `
def f4():
    return 4
`

	// SPREAD_5 is the module spread_5.star, which defines f5 returning 5.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_5.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_5 = `
def f5():
    return 5
`

	// SPREAD_6 is the module spread_6.star, which defines f6 returning 6.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, inlined from testdata/spread_6.star (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SPREAD_6 = `
def f6():
    return 6
`
)

// _Text is a script const's text without the newline that follows its opening
// backtick, which keeps the first line of the script off the line of the const.
//
// Revisions:
//   - 2026-11-09 11:37: initial creation (C114, Q289)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Text(text string) string {
	return strings.TrimPrefix(
		text,
		"\n",
	)
}

var (
	// FIXTURES serves every script of the package by module name, as the files of
	// testdata once did. The keys are the names a script loads and an error shows.
	//
	// Revisions:
	//   - 2026-11-09 11:25: initial creation, replacing the files in testdata (C114, Q281)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FIXTURES = _Sources{
		"app.star":         _Text(APP_FIXTURE),
		"lib.star":         _Text(LIB_FIXTURE),
		"helper.star":      _Text(HELPER_FIXTURE),
		"mainargs.star":    _Text(MAINARGS_FIXTURE),
		"touch.star":       _Text(TOUCH_FIXTURE),
		"spread.star":      _Text(SPREAD_FIXTURE),
		"spread_1.star":    _Text(SPREAD_1),
		"spread_2.star":    _Text(SPREAD_2),
		"spread_3.star":    _Text(SPREAD_3),
		"spread_4.star":    _Text(SPREAD_4),
		"spread_5.star":    _Text(SPREAD_5),
		"spread_6.star":    _Text(SPREAD_6),
		"cycle_a.star":     _Text(CYCLE_A_FIXTURE),
		"cycle_b.star":     _Text(CYCLE_B),
		"both.star":        _Text(BOTH),
		"left/lib.star":    _Text(LEFT_LIB),
		"left/side.star":   _Text(LEFT_SIDE),
		"right/lib.star":   _Text(RIGHT_LIB),
		"right/side.star":  _Text(RIGHT_SIDE),
		"concurrent.star":  _Text(CONCURRENT_FIXTURE),
		"forgets.star":     _Text(FORGETS_FIXTURE),
		"spins.star":       _Text(SPINS_FIXTURE),
		"maindefault.star": _Text(MAINDEFAULT_FIXTURE),
		"breaks.star":      _Text(BREAKS_FIXTURE),
		"spelling.star":    _Text(SPELLING_FIXTURE),
		"explodes.star":    _Text(EXPLODES_FIXTURE),
		"unjoined.star":    _Text(UNJOINED_FIXTURE),
	}
)

// TestMain installs the exploding plugin before any test compiles a script.
//
// The runtime's own builtins arrive by importing core, as they do for a host.
// The registry is per process, and a test binary is its own process, so the
// plugin only these tests use is installed here.
//
// Revisions:
//   - 2026-09-19 22:54: initial creation
//   - 2026-09-19 23:38: says why this stays now the facade exists, rather than
//     promising it can go
//   - 2026-09-21 09:46: the builtins are a plugin, imported like any other
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestMain(m *testing.M) {
	plugin.Register(&_Exploding{})

	os.Exit(m.Run())
}
