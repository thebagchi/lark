package script

import (
	"fmt"
	"slices"
	"strings"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// Stage names the step of a walk that stopped it.
//
// Revisions:
//   - 2026-11-02 20:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type Stage int

const (
	// STAGE_RESOLVE is the loader refusing to name a module.
	//
	// Revisions:
	//   - 2026-11-02 20:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STAGE_RESOLVE Stage = iota

	// STAGE_CYCLE is a module reached while it is still being read.
	//
	// Revisions:
	//   - 2026-11-02 20:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STAGE_CYCLE

	// STAGE_DEPTH is a chain of loads longer than MAX_DEPTH.
	//
	// Revisions:
	//   - 2026-11-02 20:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STAGE_DEPTH

	// STAGE_LOAD is the loader failing to fetch a module it named.
	//
	// Revisions:
	//   - 2026-11-02 20:49: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	STAGE_LOAD
)

// StopError is why a Walk refused to go on.
//
// The walk owns the order of its steps and the words of the refusal: Error
// writes them once, for a compile and a derivation alike.
//
// Err is the loader's error for STAGE_RESOLVE and STAGE_LOAD, and spelling's
// spelling.ErrCycle and spelling.ErrDepth for the other two. Ring is set for STAGE_CYCLE only: the
// modules from the first visit of Name to the load that closes the ring, Name
// last.
//
// Revisions:
//   - 2026-11-02 20:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
type StopError struct {
	Stage  Stage
	From   string
	Target string
	Name   string
	Ring   []string
	Err    error
}

// Walk follows the loads of a script depth first, one module at a time.
//
// Two sides read a script: a compile and a derivation. Each needs the same
// four rules - name a module before fetching it, refuse one that is still being
// read, skip one already read, refuse a chain over MAX_DEPTH - and the order of
// those rules is the point. A copy in each side drifted once already, with a
// different cycle text, and a third copy would be patchwork.
//
// A Walk holds no module text. The caller reads each module in its own
// callback and keeps what it wants.
//
// Revisions:
//   - 2026-11-02 20:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 18:30: moved under v1/runtime/internal; holds a common.Loader (C91w2)
type Walk struct {
	loader  common.Loader
	entered map[string]bool
	chain   []string
}

// NewWalk starts a walk at entry, which is on the chain and already read.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-04 23:50: a nil loader reads files beside the loading one (C86 fix round 2)
//   - 2026-11-07 18:30: takes a common.Loader (C91w2)
func NewWalk(
	loader common.Loader,
	entry string,
) *Walk {
	if loader == nil {
		loader = new(_Dir)
	}

	return &Walk{
		loader: loader,
		entered: map[string]bool{
			entry: true,
		},
		chain: []string{entry},
	}
}

// Reach brings the module target, as spelled by from, into the walk, and
// returns the name the loader gave it.
//
// The loader resolves first, because until it says what target is called there
// is nothing to compare. The chain is checked before the set of modules read:
// a module is entered before its own loads are walked, so the set answers
// "seen it" for a module still being read, which is exactly a cycle. The fetch
// comes last, so a module already read and a module that closes a cycle cost
// no read.
//
// read gets the name and the text, with the name on the chain while it runs.
// An error from read passes through untouched. Any other error is a *StopError.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43), from the walks of graph and
//     artifact
//   - 2026-11-04 23:50: a nil Walk returns ErrNoSource (C86 fix round 2)
//   - 2026-11-04 23:50: a zero Walk, which has no loader, returns ErrNoSource, and a
//     nil read function reads nothing (C86 fix round 3)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource (C121, Q348)
func (w *Walk) Reach(
	from string,
	target string,
	read func(
		name string,
		src []byte,
	) error,
) (string, error) {
	if w == nil || w.loader == nil {
		return "", ErrSource
	}

	if read == nil {
		read = func(
			string,
			[]byte,
		) error {
			return nil
		}
	}

	name, err := w.loader.Resolve(
		from,
		target,
	)
	if err != nil {
		return "", &StopError{
			Stage:  STAGE_RESOLVE,
			From:   from,
			Target: target,
			Err:    err,
		}
	}

	stop := w._Cycling(
		from,
		target,
		name,
	)
	if stop != nil {
		return "", stop
	}

	if w.entered[name] {
		return name, nil
	}

	return name, w._Fetched(
		from,
		target,
		name,
		read,
	)
}

// _Cycling is the stop for name when it is already on the chain, or nil.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
func (w *Walk) _Cycling(
	from string,
	target string,
	name string,
) *StopError {
	start := slices.Index(
		w.chain,
		name,
	)
	if start < 0 {
		return nil
	}

	return &StopError{
		Stage:  STAGE_CYCLE,
		From:   from,
		Target: target,
		Name:   name,
		Ring: append(
			slices.Clone(w.chain[start:]),
			name,
		),
		Err: spelling.ErrCycle,
	}
}

// _Fetched reads name with name on the chain while read runs.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func (w *Walk) _Fetched(
	from string,
	target string,
	name string,
	read func(
		name string,
		src []byte,
	) error,
) error {
	if len(w.chain) >= common.MAX_DEPTH {
		return &StopError{
			Stage:  STAGE_DEPTH,
			From:   from,
			Target: target,
			Name:   name,
			Err:    spelling.ErrDepth,
		}
	}

	src, err := w.loader.Load(name)
	if err != nil {
		return &StopError{
			Stage:  STAGE_LOAD,
			From:   from,
			Target: target,
			Name:   name,
			Err:    err,
		}
	}

	w.entered[name] = true
	w.chain = append(
		w.chain,
		name,
	)

	defer func() {
		w.chain = w.chain[:len(w.chain)-1]
	}()

	return read(
		name,
		src,
	)
}

// Error is the refusal in words, the same for every caller: what a compile says.
//
// A name from a script is quoted and cut, so a name with a line break or a
// great length cannot fill a log. The ring of a cycle is the loader's own
// names.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-04 23:50: a nil StopError reads as a walk that stopped with no
//     cause (C86 fix round 4)
//   - 2026-11-05 23:24: writes the words once for graph and amalgam, which
//     each wrote their own (C60, Q187)
//   - 2026-11-10 11:44: cuts every module name of the ring and each From to
//     QUOTE_LIMIT bytes (C117, B239, Q4)
func (s *StopError) Error() string {
	if s == nil {
		return "the walk of loads stopped"
	}

	switch s.Stage {
	case STAGE_RESOLVE:
		return fmt.Sprintf(
			"cannot resolve %s from %s: %v",
			unpack.QuotePrefix(s.Target),
			unpack.ShortText(s.From),
			unpack.ShortPath(s.Err),
		)
	case STAGE_CYCLE:
		return fmt.Sprintf(
			"%v: %s",
			s.Err,
			strings.Join(
				_Shortened(s.Ring),
				" -> ",
			),
		)
	case STAGE_DEPTH:
		return fmt.Sprintf(
			"%s from %s: %v",
			unpack.ShortText(s.Name),
			unpack.ShortText(s.From),
			s.Err,
		)
	default:
		return fmt.Sprintf(
			"cannot load %s from %s: %v",
			unpack.QuotePrefix(s.Target),
			unpack.ShortText(s.From),
			unpack.ShortPath(s.Err),
		)
	}
}

// Unwrap is the error under the stop.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-04 23:50: a nil StopError has no error under it (C86 fix round 4)
func (s *StopError) Unwrap() error {
	if s == nil {
		return nil
	}

	return s.Err
}

// _Shortened is names with each name cut by unpack.ShortText. The caller's
// slice stays as it is.
//
// Revisions:
//   - 2026-11-10 11:44: initial creation (C117, B239, Q4)
func _Shortened(names []string) []string {
	cut := make(
		[]string,
		len(names),
	)
	for i, name := range names {
		cut[i] = unpack.ShortText(name)
	}

	return cut
}
