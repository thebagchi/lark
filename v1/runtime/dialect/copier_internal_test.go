// This file is the internal test form, which .guidelines/conventions/go.md
// allows only when the thing under test is unexported and the file says why.
//
// Why: _Copied, the copy Compile compiles in place of a second parse, is
// unexported, and the second parse it replaced is built here from the
// unexported steps that marked it.
package dialect

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

const (
	ROOT       = "../../.."
	SCRIPTS    = ".star"
	REPOSITORY = ".git"
)

// TestCompile_SameAsTwoParses compiles every script in the repository the way
// Compile does and the way it did before, with a second parse, and checks the
// two give the same program, or the same error.
//
// Every name is predeclared, so a script resolves without the environment it
// runs in; spawn is then the builtin, which is what makes the lambdas taken.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func TestCompile_SameAsTwoParses(t *testing.T) {
	for _, path := range _Sources(t) {
		t.Run(
			path,
			func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}

				var (
					_, got, refused = Compile(
						path,
						src,
						_Everything,
					)
					want, failed = _Twice(
						path,
						src,
						_Everything,
					)
				)

				_Agree(
					t,
					refused,
					failed,
				)

				if refused != nil {
					return
				}

				if !bytes.Equal(
					_Written(
						t,
						got,
					),
					_Written(
						t,
						want,
					),
				) {
					t.Fatal("the copy compiled to another program")
				}
			},
		)
	}
}

// TestCopied_SharesNoNode checks the copy of every script in the repository
// holds as many nodes as the tree it copies, and not one of them, so that
// resolving one tree writes nothing into the other.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func TestCopied_SharesNoNode(t *testing.T) {
	for _, path := range _Sources(t) {
		t.Run(
			path,
			func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}

				written, err := OPTIONS.Parse(
					path,
					src,
					0,
				)
				if err != nil {
					t.Skip("does not parse, so there is no tree to copy")
				}

				copied, err := _Copied(written)
				if err != nil {
					t.Fatal(err)
				}

				var (
					nodes = _Nodes(written)
					fresh = _Nodes(copied)
				)

				if len(fresh) != len(nodes) {
					t.Fatalf(
						"copied %d nodes of %d",
						len(fresh),
						len(nodes),
					)
				}

				for node := range fresh {
					if nodes[node] {
						t.Fatalf(
							"the copy shares a %T",
							node,
						)
					}
				}
			},
		)
	}
}

// _Sources is every Starlark script in the repository: the samples and each
// package's test data.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Sources(t *testing.T) []string {
	t.Helper()

	var paths []string

	err := filepath.WalkDir(
		ROOT,
		func(
			path string,
			entry fs.DirEntry,
			err error,
		) error {
			if err != nil {
				return err
			}

			if entry.IsDir() && entry.Name() == REPOSITORY {
				return filepath.SkipDir
			}

			if filepath.Ext(path) == SCRIPTS {
				paths = append(
					paths,
					path,
				)
			}

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) == 0 {
		t.Fatal("found no script")
	}

	return paths
}

// _Everything predeclares every name.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Everything(string) bool {
	return true
}

// _Twice compiles src as Compile did before the copy: a second parse is
// marked and compiled, and the first is only resolved.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Twice(
	path string,
	src []byte,
	predeclared func(string) bool,
) (*starlark.Program, error) {
	written, err := OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, err
	}

	err = resolve.File(
		written,
		predeclared,
		starlark.Universe.Has,
	)
	if err != nil {
		return nil, err
	}

	compiled, err := OPTIONS.Parse(
		path,
		src,
		0,
	)
	if err != nil {
		return nil, err
	}

	_Marked(
		compiled,
		written,
	)

	return starlark.FileProgram(
		compiled,
		predeclared,
	)
}

// _Agree fails unless both compiles failed with the same text or neither did.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Agree(
	t *testing.T,
	got error,
	want error,
) {
	t.Helper()

	if got == nil && want == nil {
		return
	}

	if got == nil || want == nil || got.Error() != want.Error() {
		t.Fatalf(
			"got %v, want %v",
			got,
			want,
		)
	}
}

// _Written is the program as it is saved.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Written(
	t *testing.T,
	program *starlark.Program,
) []byte {
	t.Helper()

	var buf bytes.Buffer

	err := program.Write(&buf)
	if err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// _Nodes is every node of a tree.
//
// Revisions:
//   - 2026-10-28 22:49: initial creation
func _Nodes(file *syntax.File) map[syntax.Node]bool {
	nodes := make(map[syntax.Node]bool)

	syntax.Walk(
		file,
		func(node syntax.Node) bool {
			if node != nil {
				nodes[node] = true
			}

			return true
		},
	)

	return nodes
}
