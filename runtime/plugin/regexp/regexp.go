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
package regexp

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/runtime/plugin"
)

// ErrPattern is returned for a pattern this engine cannot read, which
// includes every one that asks for lookahead or a backreference.
var ErrPattern = errors.New("not a pattern this reads")

const (
	// NAME is the module, and the names it holds.
	NAME    = "regexp"
	MATCH   = "match"
	SEARCH  = "search"
	FINDALL = "findall"
	SUB     = "sub"
	SPLIT   = "split"
	QUOTE   = "quote"

	// TEXT, START, END, GROUPS and NAMED are what a match calls its parts.
	TEXT   = "text"
	START  = "start"
	END    = "end"
	GROUPS = "groups"
	NAMED  = "named"

	// COUNT is the optional limit sub and split take, and ALL is what it
	// means when nobody gives one.
	COUNT = "count"
	ALL   = 0

	// KEPT is how many compiled patterns are held before the lot is dropped.
	//
	// A cache keyed by something a script writes is unbounded unless somebody
	// bounds it, and a workflow that builds patterns in a loop would grow it
	// forever. Dropping all of them rather than the oldest is deliberate: what
	// this buys is not compiling the same pattern twice in a loop, and a loop
	// re-fills the cache in one pass.
	KEPT = 256
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func init() {
	plugin.Register(new(_Regexp))
}

// _Regexp is the plugin, and the cache of what it has already compiled.
//
// The cache is the plugin's rather than a run's: a compiled pattern is the
// same pattern whoever asks, holds nothing about who asked, and is safe for
// any number of goroutines by the standard library's own promise.
type _Regexp struct {
	guard sync.Mutex
	held  map[string]*regexp.Regexp
}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) Name() string {
	return NAME
}

// Values returns the regexp module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				MATCH:   starlark.NewBuiltin(NAME+"."+MATCH, r._Match),
				SEARCH:  starlark.NewBuiltin(NAME+"."+SEARCH, r._Search),
				FINDALL: starlark.NewBuiltin(NAME+"."+FINDALL, r._FindAll),
				SUB:     starlark.NewBuiltin(NAME+"."+SUB, r._Sub),
				SPLIT:   starlark.NewBuiltin(NAME+"."+SPLIT, r._Split),
				QUOTE:   starlark.NewBuiltin(NAME+"."+QUOTE, _Quote),
			},
		},
	}
}

// _Compiled is the pattern, compiled once however often it is asked for.
//
// Returns ErrPattern for one this engine cannot read.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Compiled(who string, pattern string) (*regexp.Regexp, error) {
	r.guard.Lock()
	defer r.guard.Unlock()

	found, known := r.held[pattern]
	if known {
		return found, nil
	}

	made, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w: %w", who, pattern, ErrPattern, err)
	}

	if r.held == nil || len(r.held) >= KEPT {
		r.held = make(map[string]*regexp.Regexp, KEPT)
	}

	r.held[pattern] = made

	return made, nil
}

// _Match is the match at the start of the text, or None.
//
// Anchored the way Python's match is: a pattern that could match later but not
// at the start finds nothing here. RE2 reports the leftmost match, so a match
// that does not begin at zero is a match this did not want.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Match(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r._Given(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	found := held.FindStringSubmatchIndex(text)
	if found == nil || found[0] != 0 {
		return starlark.None, nil
	}

	return _Match(held, text, found), nil
}

// _Search is the first match anywhere in the text, or None.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Search(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r._Given(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	found := held.FindStringSubmatchIndex(text)
	if found == nil {
		return starlark.None, nil
	}

	return _Match(held, text, found), nil
}

// _FindAll is every match, in the order they appear.
//
// A list of the same matches search returns, rather than of strings, so that a
// caller reading one reads it the same way whichever call produced it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _FindAll(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r._Given(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	every := held.FindAllStringSubmatchIndex(text, -1)

	found := make([]starlark.Value, 0, len(every))

	for _, one := range every {
		found = append(found, _Match(held, text, one))
	}

	return starlark.NewList(found), nil
}

// _Sub is the text with every match replaced, or the first count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Sub(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		pattern string
		repl    string
		text    string
		count   = ALL
	)

	err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"pattern", &pattern, "repl", &repl, "text", &text, COUNT+"?", &count)
	if err != nil {
		return nil, err
	}

	held, err := r._Compiled(fn.Name(), pattern)
	if err != nil {
		return nil, err
	}

	if count <= ALL {
		return starlark.String(held.ReplaceAllString(text, repl)), nil
	}

	return starlark.String(_Limited(held, text, repl, count)), nil
}

// _Limited replaces the first count matches and leaves the rest.
//
// The standard library replaces all or none, so a count is done here: the
// matches are found by position, and the text is rebuilt around the first
// count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Limited(held *regexp.Regexp, text string, repl string, count int) string {
	every := held.FindAllStringSubmatchIndex(text, count)

	var (
		out  []byte
		last int
	)

	for _, one := range every {
		out = append(out, text[last:one[0]]...)
		out = held.ExpandString(out, repl, text, one)
		last = one[1]
	}

	return string(out) + text[last:]
}

// _Split is the text cut at every match, or at the first count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Split(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		pattern string
		text    string
		count   = ALL
	)

	err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"pattern", &pattern, "text", &text, COUNT+"?", &count)
	if err != nil {
		return nil, err
	}

	held, err := r._Compiled(fn.Name(), pattern)
	if err != nil {
		return nil, err
	}

	// The standard library counts pieces where this counts cuts, and reads a
	// negative as "every one".
	limit := -1
	if count > ALL {
		limit = count + 1
	}

	parts := held.Split(text, limit)

	found := make([]starlark.Value, 0, len(parts))

	for _, part := range parts {
		found = append(found, starlark.String(part))
	}

	return starlark.NewList(found), nil
}

// _Quote is text with every special character escaped, so that it matches
// itself and nothing else.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Quote(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var text string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &text)
	if err != nil {
		return nil, err
	}

	return starlark.String(regexp.QuoteMeta(text)), nil
}

// _Given reads the pattern and the text every finder takes, and compiles the
// first.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Regexp) _Given(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*regexp.Regexp, string, error) {
	var pattern, text string

	err := starlark.UnpackArgs(fn.Name(), args, kwargs, "pattern", &pattern, "text", &text)
	if err != nil {
		return nil, "", err
	}

	held, err := r._Compiled(fn.Name(), pattern)
	if err != nil {
		return nil, "", err
	}

	return held, text, nil
}

// _Match is one match as a script reads it: what matched, where, and what the
// groups caught.
//
// start and end are byte offsets, which is what the engine counts in. A script
// slicing the subject with them gets what matched; a script counting
// characters in a subject that is not ASCII will find they differ.
//
// A group that took part in no match is None rather than the empty string,
// because a group that matched nothing and a group that matched an empty
// string are different answers.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Match(held *regexp.Regexp, text string, found []int) starlark.Value {
	groups := make([]starlark.Value, 0, len(found)/2-1)
	named := starlark.NewDict(len(found) / 2)

	for index := 1; index < len(found)/2; index++ {
		caught := _Caught(text, found, index)

		groups = append(groups, caught)

		name := held.SubexpNames()[index]
		if name == "" {
			continue
		}

		// Setting a key on a dict this function owns cannot fail: the key is a
		// string, which is hashable, and nothing has frozen it.
		err := named.SetKey(starlark.String(name), caught)
		if err != nil {
			continue
		}
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		TEXT:   starlark.String(text[found[0]:found[1]]),
		START:  starlark.MakeInt(found[0]),
		END:    starlark.MakeInt(found[1]),
		GROUPS: starlark.NewList(groups),
		NAMED:  named,
	})
}

// _Caught is what one group caught, or None where it took part in no match.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Caught(text string, found []int, index int) starlark.Value {
	at, to := found[2*index], found[2*index+1]
	if at < 0 {
		return starlark.None
	}

	return starlark.String(text[at:to])
}
