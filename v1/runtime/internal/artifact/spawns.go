package artifact

import (
	"go.starlark.net/syntax"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// Spawns is every spawn the entry script writes inside a def, in the order it
// writes them: the def it is written in and the function it starts.
//
// A run that ended early reads this to find the spawns the stop kept it from
// making. A spawn names a function directly - spawn(work) - or through a lambda
// whose whole body is one call - spawn(lambda: work(1)); a spawn of anything
// else names no function and is left out, as is one outside every def. A name
// counts only when the script defines a function of it at its top level, so
// spawn(f), with f a variable, is left out.
//
// A compiled artifact reads its merged tree without the definitions the merge
// shook out, since a spawn in a function nothing calls is no spawn a run makes.
// A loaded one has no tree, and reads the merged source its bundle carries
// instead: the same text, parsed again, so the sites are the same. This is why
// a bundle stores no list of them. A bundle written before the source was
// carried has none to read, and reports no spawn; a source that does not parse
// reports none either. Nothing is stored on the artifact, so a run that
// succeeded never pays for it.
//
// The source of a bundle is not checked against its code. It is text for a
// reader, and a bundle altered to say another source shows another graph, and
// runs the same code.
//
// A nil or zero artifact reports none.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-05 17:15: reads the tree without the shaken definitions, as the
//     bundle's source does (C44b, Q157)
func (a *Artifact) Spawns() []common.Spawn {
	entry := a._Entry()
	if entry == nil {
		return nil
	}

	tree := entry.kept
	if tree == nil {
		tree = _Reparsed(entry.saved)
	}

	if tree == nil {
		return nil
	}

	return _Written(tree)
}

// _Reparsed is the tree of the source a saved unit carries, or nil when the
// unit has none or it does not parse.
//
// The parser refuses nesting past 1000 levels, so untrusted text cannot
// exhaust the stack, and the tree takes memory in proportion to the text.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func _Reparsed(unit *artifactpb.Unit) *syntax.File {
	if unit.GetSource() == "" {
		return nil
	}

	tree, err := common.OPTIONS.Parse(
		unit.GetName(),
		unit.GetSource(),
		0,
	)
	if err != nil {
		return nil
	}

	return tree
}

// _Written reads every spawn call of tree that sits inside a def and starts a
// function.
//
// The def a call is in is the nearest one above it, so a spawn inside a lambda
// belongs to the def holding the lambda. A target counts when it is a function
// the script defines at its top level.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
func _Written(tree *syntax.File) []common.Spawn {
	var (
		sites []common.Spawn
		stack []syntax.Node
		known = _Toplevel(tree)
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]

				return true
			}

			stack = append(
				stack,
				node,
			)

			var (
				target     = _Spawned(node)
				caller     = _Inside(stack)
				_, defined = known[target]
			)

			if defined && caller != "" {
				sites = append(
					sites,
					common.Spawn{
						Caller: caller,
						Target: target,
					},
				)
			}

			return true
		},
	)

	return sites
}

// _Toplevel is the name of each function tree defines at its top level.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
func _Toplevel(tree *syntax.File) map[string]struct{} {
	known := make(map[string]struct{})

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok {
			known[def.Name.Name] = struct{}{}
		}
	}

	return known
}

// _Spawned is the function node starts when it is a spawn call, or empty.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
func _Spawned(node syntax.Node) string {
	call, ok := node.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}

	fn, ok := call.Fn.(*syntax.Ident)
	if !ok || fn.Name != common.SPAWN {
		return ""
	}

	arg := call.Args[0]

	if lambda, ok := arg.(*syntax.LambdaExpr); ok {
		body, ok := lambda.Body.(*syntax.CallExpr)
		if !ok {
			return ""
		}

		arg = body.Fn
	}

	name, ok := arg.(*syntax.Ident)
	if !ok {
		return ""
	}

	return name.Name
}

// _Inside is the name of the nearest def on stack, or empty.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
func _Inside(stack []syntax.Node) string {
	for idx := len(stack) - 1; idx >= 0; idx-- {
		def, ok := stack[idx].(*syntax.DefStmt)
		if ok {
			return def.Name.Name
		}
	}

	return ""
}
