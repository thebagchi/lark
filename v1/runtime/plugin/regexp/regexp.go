// Package regexp gives a script regular expressions. Importing it is what
// enables it.
//
// RE2, which is the standard library's engine, and the choice is about safety
// rather than taste: RE2 matches in time linear in the length of the subject,
// so no pattern a script can write makes a run hang. The engines that offer
// lookahead, lookbehind and backreferences buy them with backtracking, and
// backtracking is a denial of service wearing a feature's clothes in anything
// that runs scripts it did not write.
//
// So those three are absent and cannot be added. Named groups - (?P<name>x) -
// are present, which is the Python spelling and the one RE2 accepts.
//
// A replacement names a group the engine's own way, $1 and ${name}, rather
// than Python's \1. One syntax, the one the engine reads, so nothing here
// rewrites a replacement on the way through and there is no second spelling to
// get wrong. A literal dollar is $$.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package regexp

import (
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/regexp/internal/match"
)

var (
	// ErrPattern is match.ErrPattern.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrPattern = match.ErrPattern
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 11:32: uses match.Regexp, which moved to internal/match (C120b, Q315)
func init() {
	plugin.Register(new(match.Regexp))
}
