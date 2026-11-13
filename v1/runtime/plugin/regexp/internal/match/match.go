// Package match holds the regexp plugin: its Regexp type, which compiles a
// pattern and serves the match, find, sub and split builtins, with the bounds on
// pattern size and match count. Package regexp keeps the registration and the
// sentinel.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package match

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/common"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/regexp/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrPattern is returned for a pattern this engine cannot read, which
// includes every one that asks for lookahead or a backreference.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
var (
	ErrPattern = errors.New("not a pattern this reads")
)

// Regexp is the plugin, and the cache of what it has already compiled.
//
// The cache is the plugin's rather than a run's: a compiled pattern is the
// same pattern whoever asks, holds nothing about who asked, and is safe for
// any number of goroutines by the standard library's own promise.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp and renamed _Regexp to Regexp, for
//     the tests in tests/ (C120b, Q315, Q314)
type Regexp struct {
	guard sync.Mutex
	held  map[string]*regexp.Regexp
}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) Name() string {
	return spec.NAME
}

// Values returns the regexp module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.MATCH: starlark.NewBuiltin(
					spec.NAME+"."+spec.MATCH,
					r._Match,
				),
				spec.SEARCH: starlark.NewBuiltin(
					spec.NAME+"."+spec.SEARCH,
					r._Search,
				),
				spec.FINDALL: starlark.NewBuiltin(
					spec.NAME+"."+spec.FINDALL,
					r._FindAll,
				),
				spec.SUB: starlark.NewBuiltin(
					spec.NAME+"."+spec.SUB,
					r.Sub,
				),
				spec.SPLIT: starlark.NewBuiltin(
					spec.NAME+"."+spec.SPLIT,
					r.Split,
				),
				spec.QUOTE: starlark.NewBuiltin(
					spec.NAME+"."+spec.QUOTE,
					Quote,
				),
			},
		},
	}
}

// Sub is the text with every match replaced, or the first count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp and renamed _Sub to Sub, for the
//     tests in tests/ (C120b, Q315, Q314)
func (r *Regexp) Sub(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		pattern string
		repl    string
		text    string
		count   = spec.ALL
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		"pattern",
		&pattern,
		"repl",
		&repl,
		"text",
		&text,
		spec.COUNT+"?",
		&count,
	)
	if err != nil {
		return nil, err
	}

	held, err := r._Compiled(
		shared.Called(fn),
		pattern,
	)
	if err != nil {
		return nil, err
	}

	if count <= spec.ALL {
		return starlark.String(held.ReplaceAllString(
			text,
			repl,
		)), nil
	}

	return starlark.String(_Limited(
		held,
		text,
		repl,
		count,
	)), nil
}

// Split is the text cut at every match, or at the first count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp and renamed _Split to Split, for
//     the tests in tests/ (C120b, Q315, Q314)
func (r *Regexp) Split(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		pattern string
		text    string
		count   = spec.ALL
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		"pattern",
		&pattern,
		"text",
		&text,
		spec.COUNT+"?",
		&count,
	)
	if err != nil {
		return nil, err
	}

	held, err := r._Compiled(
		shared.Called(fn),
		pattern,
	)
	if err != nil {
		return nil, err
	}

	// The standard library counts pieces where this counts cuts, and reads a
	// negative as "every one".
	limit := -1
	if count > spec.ALL {
		limit = count + 1
	}

	return _Listed(
		held.Split(
			text,
			limit,
		),
	), nil
}

// Given reads the pattern and the text every finder takes, and compiles the
// first.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp and renamed _Given to Given, for
//     the tests in tests/ (C120b, Q315, Q314)
func (r *Regexp) Given(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*regexp.Regexp, string, error) {
	var pattern, text string

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		"pattern",
		&pattern,
		"text",
		&text,
	)
	if err != nil {
		return nil, "", err
	}

	held, err := r._Compiled(
		shared.Called(fn),
		pattern,
	)
	if err != nil {
		return nil, "", err
	}

	return held, text, nil
}

// _Compiled is the pattern, compiled once however often it is asked for.
//
// Returns ErrPattern for one this engine cannot read. The text of the error
// quotes at most common.QUOTE_LIMIT bytes of the pattern.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 10:57: quote a bounded prefix of the pattern, and bound the
//     standard library's own copy of it with _Bounded
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) _Compiled(
	who string,
	pattern string,
) (*regexp.Regexp, error) {
	r.guard.Lock()
	defer r.guard.Unlock()

	found, known := r.held[pattern]
	if known {
		return found, nil
	}

	made, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf(
			"%s got %s: %w: %w",
			who,
			unpack.QuotePrefix(pattern),
			ErrPattern,
			_Bounded(err),
		)
	}

	if r.held == nil || len(r.held) >= spec.KEPT {
		r.held = make(
			map[string]*regexp.Regexp,
			spec.KEPT,
		)
	}

	r.held[pattern] = made

	return made, nil
}

// _Bounded is err with the pattern it repeats cut to a short prefix.
//
// The standard library puts the whole pattern, or the fragment that failed,
// into a *syntax.Error: a 5,000,001-byte pattern gave an error of 5,000,044
// bytes. Quoting the pattern once with unpack.QuotePrefix does not bound that second
// copy, so this builds the error again with the same code and a quoted prefix
// of Expr. An error that holds common.QUOTE_LIMIT bytes or fewer, or that is not a
// *syntax.Error, comes back as it is, so a short pattern keeps the standard
// text byte for byte. A long one swaps the backticks of the standard text for
// double quotes.
//
// Revisions:
//   - 2026-11-02 10:57: initial creation, for decision Q4 (change C21)
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Bounded(err error) error {
	var held *syntax.Error
	if !errors.As(
		err,
		&held,
	) || len(held.Expr) <= common.QUOTE_LIMIT {
		return err
	}

	return fmt.Errorf(
		"error parsing regexp: %s: %s",
		held.Code,
		unpack.QuotePrefix(held.Expr),
	)
}

// _Match is the match at the start of the text, or None.
//
// Anchored the way Python's match is: a pattern that could match later but not
// at the start finds nothing here. RE2 reports the leftmost match, so a match
// that does not begin at zero is a match this did not want.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) _Match(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r.Given(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	found := held.FindStringSubmatchIndex(text)
	if found == nil || found[0] != 0 {
		return starlark.None, nil
	}

	return _Match(
		held,
		text,
		found,
	), nil
}

// _Search is the first match anywhere in the text, or None.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) _Search(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r.Given(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	found := held.FindStringSubmatchIndex(text)
	if found == nil {
		return starlark.None, nil
	}

	return _Match(
		held,
		text,
		found,
	), nil
}

// _FindAll is every match, in the order they appear.
//
// A list of the same matches search returns, rather than of strings, so that a
// caller reading one reads it the same way whichever call produced it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func (r *Regexp) _FindAll(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, text, err := r.Given(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	var (
		every = held.FindAllStringSubmatchIndex(
			text,
			-1,
		)
		found = make(
			[]starlark.Value,
			0,
			len(every),
		)
	)

	for _, one := range every {
		found = append(
			found,
			_Match(
				held,
				text,
				one,
			),
		)
	}

	return starlark.NewList(found), nil
}

// _Limited replaces the first count matches and leaves the rest.
//
// The standard library replaces all or none, so a count is done here: the
// matches are found by position, and the text is rebuilt around the first
// count of them.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Limited(
	held *regexp.Regexp,
	text string,
	repl string,
	count int,
) string {
	every := held.FindAllStringSubmatchIndex(
		text,
		count,
	)

	var (
		out  []byte
		last int
	)

	for _, one := range every {
		out = append(
			out,
			text[last:one[0]]...,
		)
		out = held.ExpandString(
			out,
			repl,
			text,
			one,
		)
		last = one[1]
	}

	return string(out) + text[last:]
}

// _Listed is the pieces of a split, as a list of strings.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Listed(parts []string) starlark.Value {
	found := make(
		[]starlark.Value,
		0,
		len(parts),
	)

	for _, part := range parts {
		found = append(
			found,
			starlark.String(part),
		)
	}

	return starlark.NewList(found)
}

// Quote is text with every special character escaped, so that it matches
// itself and nothing else.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp and renamed _Quote to Quote, for
//     the tests in tests/ (C120b, Q315, Q314)
func Quote(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var text string

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&text,
	)
	if err != nil {
		return nil, err
	}

	return starlark.String(regexp.QuoteMeta(text)), nil
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
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Match(
	held *regexp.Regexp,
	text string,
	found []int,
) starlark.Value {
	var (
		count  = len(found) / spec.BOUNDS
		groups = make(
			[]starlark.Value,
			0,
			count-1,
		)
		named = starlark.NewDict(count)
	)

	filled := &_Fill{
		count:  count,
		groups: groups,
		named:  named,
	}

	_Groups(
		held,
		text,
		found,
		filled,
	)

	return starlarkstruct.FromStringDict(
		starlarkstruct.Default,
		starlark.StringDict{
			spec.TEXT:   starlark.String(text[found[0]:found[1]]),
			spec.START:  starlark.MakeInt(found[0]),
			spec.END:    starlark.MakeInt(found[1]),
			spec.GROUPS: starlark.NewList(filled.groups),
			spec.NAMED:  filled.named,
		},
	)
}

// _Fill is the groups one match is filling in.
//
// Revisions:
//   - 2026-10-28 18:20: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
type _Fill struct {
	count  int
	groups []starlark.Value
	named  *starlark.Dict
}

// _Groups records each captured group, by position and by name.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Groups(
	held *regexp.Regexp,
	text string,
	found []int,
	match *_Fill,
) {
	for index := 1; index < match.count; index++ {
		caught := _Caught(
			text,
			found,
			index,
		)

		match.groups = append(
			match.groups,
			caught,
		)

		name := held.SubexpNames()[index]
		if name == "" {
			continue
		}

		err := match.named.SetKey(
			starlark.String(name),
			caught,
		)
		if err != nil {
			continue
		}
	}
}

// _Caught is what one group caught, or None where it took part in no match.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/regexp (C120b, Q315)
func _Caught(
	text string,
	found []int,
	index int,
) starlark.Value {
	at, to := found[2*index], found[2*index+1]
	if at < 0 {
		return starlark.None
	}

	return starlark.String(text[at:to])
}
