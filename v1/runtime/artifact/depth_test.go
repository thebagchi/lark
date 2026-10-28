package artifact_test

import (
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/script"
)

const (
	DEPTH_CHAIN = 257
)

// _Chain is a Loader that serves a linear chain of modules: module 0 loads
// module 1, module 1 loads module 2, and so on up to DEPTH_CHAIN - 1. The
// last module loads nothing.
type _Chain struct {
	depth int
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
func (c *_Chain) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the source of the module at name: a load of the next module
// in the chain, or nothing when the chain ends.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
func (c *_Chain) Load(name string) ([]byte, error) {
	var idx int

	_, err := fmt.Sscanf(
		name,
		"m%d.star",
		&idx,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"unexpected module name %s",
			name,
		)
	}

	if idx >= c.depth-1 {
		return []byte("x = 1\n"), nil
	}

	return []byte(fmt.Sprintf(
		"load(\"m%d.star\", \"x\")\n",
		idx+1,
	)), nil
}

// TestRegression_Depth proves a chain longer than MAX_DEPTH returns an error
// instead of overflowing the stack. The test process must stay up.
//
// Revisions:
//   - 2026-10-24 20:00: initial creation
func TestRegression_Depth(t *testing.T) {
	var (
		entry = "entry.star"
		text  = []byte(
			"load(\"m0.star\", \"x\")\n\n" +
				"def main():\n    pass\n",
		)
		loader = &_Chain{
			depth: DEPTH_CHAIN,
		}
		source = &script.Source{
			Entry:  entry,
			Text:   text,
			Loader: loader,
		}
	)

	_, err := artifact.Compile(source)
	if !errors.Is(
		err,
		artifact.ErrDepth,
	) {
		t.Fatalf(
			"got %v, want ErrDepth",
			err,
		)
	}

	t.Logf(
		"refused: %v",
		err,
	)
}
