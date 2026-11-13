package merge

import (
	"fmt"
	"strings"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Names decides the final spelling of every global.
//
// Three steps. First each load that renames asks for its local name, the first
// load in walk order winning. Then globals are grouped by the spelling they
// ask for: a group of more than one, or a spelling a builtin has, puts the
// module prefix on every member, except the entry's main, which keeps its
// name. A group of one keeps its spelling.
//
// Entry first, then modules in walk order, each module's globals in source
// order, so the result depends on the text and never on a map walk.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept
//   - 2026-11-02 22:58: finds the builtins and the spellings in use only when
//     they can matter
func (b *_Build) _Names(entry *_Module) error {
	asked, err := b._Asked()
	if err != nil {
		return err
	}

	// Only a file besides the first can read a name as a builtin that a global
	// of another file also has, and only a clash needs the spellings in use.
	var (
		builtin map[string]bool
		taken   map[string]bool
	)

	if len(b.order) > 1 {
		builtin = b._Builtin()
	}

	groups, spelled := _Grouped(
		append(
			[]*_Module{entry},
			b.order[:len(b.order)-1]...,
		),
		entry,
		asked,
	)

	for _, name := range spelled {
		if len(groups[name]) == 1 && !builtin[name] {
			groups[name][0].mod.final[groups[name][0].bind.First] = name
		}
	}

	for _, name := range spelled {
		if len(groups[name]) == 1 && !builtin[name] {
			continue
		}

		if taken == nil {
			taken = b._Spellings()
		}

		for _, owner := range groups[name] {
			owner._Prefix(
				name,
				taken,
			)
		}
	}

	return nil
}

// _Grouped is the globals of mods grouped by the spelling each asks for, and
// those spellings in the order they were first asked.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func _Grouped(
	mods []*_Module,
	entry *_Module,
	asked map[*syntax.Ident]string,
) (map[string][]*_Owner, []string) {
	var (
		groups  = map[string][]*_Owner{}
		spelled []string
	)

	for _, mod := range mods {
		for _, bind := range mod.order {
			name := bind.First.Name
			if want, renamed := asked[bind.First]; renamed {
				name = want
			}

			if groups[name] == nil {
				spelled = append(
					spelled,
					name,
				)
			}

			groups[name] = append(
				groups[name],
				&_Owner{
					mod:  mod,
					bind: bind,
					pin:  mod == entry && bind.First.Name == common.ENTRY,
				},
			)
		}
	}

	return groups, spelled
}

// _Prefix gives the global the spelling that puts its module's prefix on name,
// unless it is pinned to name.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func (o *_Owner) _Prefix(
	name string,
	taken map[string]bool,
) {
	spelt := name
	if !o.pin {
		spelt = _Fresh(
			name,
			o.mod.name,
			taken,
		)
	}

	taken[spelt] = true
	o.mod.final[o.bind.First] = spelt
}

// _Owner is one global and the module that declares it.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Owner struct {
	mod  *_Module
	bind *resolve.Binding
	pin  bool
}

// _Asked is the spelling each global asks for, by the first load in walk order
// that renames it.
//
// A load asks for its local name only when no function of the declaring module
// has a local of that spelling, since the global would then capture it. Such a
// load is substituted at its own uses instead, which _Loaded checks.
//
// Returns ErrExport for a load of a name the module does not define.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-10 17:15: uses ErrExport, formerly ErrNoExport (C121, Q348)
func (b *_Build) _Asked() (map[*syntax.Ident]string, error) {
	asked := map[*syntax.Ident]string{}

	for _, mod := range b.order {
		for idx, load := range mod.loads {
			target := mod.targets[idx]

			for pos, from := range load.From {
				bind, ok := target.globals[from.Name]
				if !ok {
					return nil, fmt.Errorf(
						"%s in %s: %w",
						unpack.ShortText(from.Name),
						unpack.QuotePrefix(target.name),
						ErrExport,
					)
				}

				var (
					to      = load.To[pos].Name
					_, done = asked[bind.First]
				)

				if to == from.Name || done || target._Local()[to] {
					continue
				}

				asked[bind.First] = to
			}
		}
	}

	return asked, nil
}

// _Builtin is every spelling any file reads as predeclared or universal.
//
// A name only the environment has, and no file reads, is no clash: nothing in
// the merge would change meaning if a global took it.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept
func (b *_Build) _Builtin() map[string]bool {
	builtin := map[string]bool{}

	for _, mod := range b.order {
		syntax.Walk(
			mod.tree,
			func(node syntax.Node) bool {
				id, ok := node.(*syntax.Ident)
				if !ok {
					return true
				}

				bind, _ := id.Binding.(*resolve.Binding)
				if _Supplied(bind) {
					builtin[id.Name] = true
				}

				return true
			},
		)
	}

	return builtin
}

// _Spellings is every spelling written anywhere in the program and in the
// environment, so a fresh name is one that nothing can capture.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept
func (b *_Build) _Spellings() map[string]bool {
	taken := map[string]bool{}

	for name := range b.env {
		taken[name] = true
	}

	for _, mod := range b.order {
		syntax.Walk(
			mod.tree,
			func(node syntax.Node) bool {
				id, ok := node.(*syntax.Ident)
				if ok {
					taken[id.Name] = true
				}

				return true
			},
		)
	}

	return taken
}

// _Locals is every spelling a function, parameter or comprehension of the tree
// binds below the top level.
//
// A name a load binds is not one: it is the file's own name for a global of
// another file, and a rename of that global is meant to reach it. Counting it
// would refuse a file that loads one export under two spellings, as a capture
// of one by the other.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), from the proof of concept
//   - 2026-11-02 22:58: leaves out the names a load binds
func _Locals(tree *syntax.File) map[string]bool {
	var (
		names  = map[string]bool{}
		loaded = _Bound(tree)
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			id, ok := node.(*syntax.Ident)
			if !ok {
				return true
			}

			bind, _ := id.Binding.(*resolve.Binding)
			if _Inner(bind) && !loaded[bind.First] {
				names[id.Name] = true
			}

			return true
		},
	)

	return names
}

// _Fresh is the name joined to the stem of the module, with one underscore
// between, and numbered when that is taken too. For _fmt in ma.star it is
// ma__fmt.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a), the prefix in front where the
//     proof of concept put the stem behind
func _Fresh(
	name string,
	module string,
	taken map[string]bool,
) string {
	stem := module[strings.LastIndex(
		module,
		"/",
	)+1:]
	stem = strings.TrimSuffix(
		stem,
		".star",
	)
	stem = strings.Map(
		func(r rune) rune {
			if _Word(r) {
				return r
			}

			return '_'
		},
		stem,
	)

	if stem == "" || stem[0] >= '0' && stem[0] <= '9' {
		stem = "m" + stem
	}

	spelt := stem + "_" + name
	for idx := 2; taken[spelt]; idx++ {
		spelt = fmt.Sprintf(
			"%s_%s_%d",
			stem,
			name,
			idx,
		)
	}

	return spelt
}

// _Supplied is true for a binding the environment or the language supplies.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func _Supplied(bind *resolve.Binding) bool {
	return bind != nil &&
		(bind.Scope == resolve.Predeclared ||
			bind.Scope == resolve.Universal)
}

// _Inner is true for a binding made below the top level.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func _Inner(bind *resolve.Binding) bool {
	return bind != nil &&
		(bind.Scope == resolve.Local ||
			bind.Scope == resolve.Cell ||
			bind.Scope == resolve.Free)
}

// _Word is true for a rune an identifier may hold.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func _Word(r rune) bool {
	return r == '_' ||
		r >= '0' && r <= '9' ||
		r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z'
}

// _Bound is the identifiers the loads of tree bind, which every use of the
// name shares as the First of its binding.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
func _Bound(tree *syntax.File) map[*syntax.Ident]bool {
	loaded := map[*syntax.Ident]bool{}

	for _, stmt := range tree.Stmts {
		load, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}

		for _, to := range load.To {
			bind, _ := to.Binding.(*resolve.Binding)
			if bind != nil {
				loaded[bind.First] = true
			}
		}
	}

	return loaded
}
