// Package utils gives a script the utils module of section 10.8 of the brief,
// which holds one thing: the local time as text. Importing it is what enables
// it.
package utils

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	NAME     = "utils"
	DATETIME = "datetime"

	// LAYOUT is the brief's YYYY-MM-DD HH:MM:SS.uuuuuu, in the reference time
	// Go writes a layout as.
	LAYOUT = "2006-01-02 15:04:05.000000"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func init() {
	plugin.Register(&_Utils{})
}

// _Utils is the plugin. Empty: it reads the clock and nothing else.
type _Utils struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func (u *_Utils) Name() string {
	return NAME
}

// Values returns the utils module.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func (u *_Utils) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				DATETIME: starlark.NewBuiltin(NAME+"."+DATETIME, _Datetime),
			},
		},
	}
}

// _Datetime is the current local time as YYYY-MM-DD HH:MM:SS.uuuuuu.
//
// It returns the text rather than printing it, which is where this departs
// from the brief's print_datetime. A script that wants the line on its log
// writes print(utils.datetime()), and one that wants the same text in a state
// entry, a filename or a message can have it too - which a function that only
// printed could not give. One function, both uses, and the script says which.
//
// Local time, as the brief asks. That makes the text depend on the machine,
// which is why nothing in this repository compares it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as print_datetime
//   - 2026-09-21 15:25: returns the text rather than printing it, and is named
//     for what it gives back
func _Datetime(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.String(time.Now().Format(LAYOUT)), nil
}
