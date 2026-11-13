package artifact

import (
	"fmt"
	"slices"
	"sync"

	"github.com/thebagchi/lark/internal/declared"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Declaring is the names the arg() calls of an artifact's script declare, read
// the first time a run is handed arguments and kept for every run after it.
// known is false when the script could not be read as a flow, which is when no
// name can be said to be missing.
//
// Revisions:
//   - 2026-11-05 23:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Declaring struct {
	once  sync.Once
	names []string
	known bool
}

// _Declared is the names the merged script declares with arg(), sorted, and
// whether they could be read.
//
// The text is the merge a compile made, or the entry unit's source in a bundle,
// which Save wrote as that same merge. Every arg() call at its top level
// declares, assigned or not (Q193), as declared.Of reads them. A script it
// refuses has no names to check against, so a run handed arguments is not
// refused for it.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71, Q170)
//   - 2026-11-05 22:10: reads every top-level arg() call, not the flow's
//     assigned ones (Q193)
func (a *Artifact) _Declared() ([]string, bool) {
	if a.names == nil {
		return nil, false
	}

	a.names.once.Do(func() {
		text, ok := a._Text()
		if !ok {
			return
		}

		names, err := declared.Of(
			a.saved.GetEntry(),
			text,
		)
		if err != nil {
			return
		}

		a.names.names = names

		a.names.known = true
	})

	return a.names.names, a.names.known
}

// _Text is the source of the entry: the merge of a compiled artifact, or the
// text a bundle carries.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71)
func (a *Artifact) _Text() ([]byte, bool) {
	if a.merged != nil {
		text, err := a._Source()

		return text, err == nil
	}

	entry := a.units[a.saved.GetEntry()]
	if entry == nil || entry.saved.GetSource() == "" {
		return nil, false
	}

	return []byte(entry.saved.GetSource()), true
}

// _Unknown refuses a run handed a key no arg() of the script declares.
//
// Checked once the units have initialised, so an error of the units' own, such
// as an arg() called in a body, is reported first, and before the entry point
// is called, so a typo costs no run. It reports the first
// unknown key in sorted order, quoted and cut (Q4), with the declared name
// nearest to it when one is close (Q158). A run handed no arguments is not
// checked, and nor is one whose script could not be read for its names.
//
// Returns an error wrapping spelling.ErrArity.
//
// Revisions:
//   - 2026-11-05 18:55: initial creation (C71, Q170, Q185)
//   - 2026-11-05 19:10: takes the arguments, checked after the units initialise
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-13 13:04: the arguments are workflow.Value (C132, Q437)
func (a *Artifact) _Unknown(fields map[string]*workflowpb.Value) error {
	if len(fields) == 0 {
		return nil
	}

	declared, known := a._Declared()
	if !known {
		return nil
	}

	var unknown []string

	for key := range fields {
		if !slices.Contains(
			declared,
			key,
		) {
			unknown = append(
				unknown,
				key,
			)
		}
	}

	if len(unknown) == 0 {
		return nil
	}

	slices.Sort(unknown)

	var hint string

	near := spelling.Nearest(
		unknown[0],
		declared,
	)
	if near != "" {
		hint = fmt.Sprintf(
			" (did you mean %s?)",
			unpack.QuotePrefix(near),
		)
	}

	return fmt.Errorf(
		"argument %s: %w: the script declares no such argument%s",
		unpack.QuotePrefix(unknown[0]),
		spelling.ErrArity,
		hint,
	)
}
