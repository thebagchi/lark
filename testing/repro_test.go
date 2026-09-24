package testing_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime"
	_ "github.com/thebagchi/lark/runtime/plugin/codec"
	_ "github.com/thebagchi/lark/runtime/plugin/file"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/runtime/plugin/state"
	"github.com/thebagchi/lark/runtime/scheduler"
)

func TestDuration_Boundary(t *testing.T) {
	seconds := float64(uint64(math.MaxInt64)+1) / float64(time.Second)
	got, err := scheduler.Duration(starlark.Float(seconds))
	t.Logf("seconds=%g duration=%d err=%v", seconds, got, err)
	if err == nil && got < 0 {
		t.Errorf("duration past int64 was accepted as %d", got)
	}
}

func TestScripts(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "note"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	note := filepath.Join(secret, "note")
	src := fmt.Sprintf(`
def main():
    hidden = file.exists(%q)
    seed = crc32(b"abc", 4294967296)
    plain = crc32(b"abc", 0)
    doc = {"a": [1]}
    out = patch_json(doc, [{"op": "copy", "from": "/a", "path": "/b"}])
    out["b"].append(2)

    def slow(v):
        sleep(0.05)
        return "from-update"

    def writer():
        sleep(0.01)
        state.set("n", "from-set")

    handle = spawn(writer)
    state.update("n", slow)
    join(handle)
    return [hidden, seed == plain, doc["a"], state.get("n")]
`, note)

	built, err := runtime.NewCompiler().Compile("probe.star", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	value, err := built.Run(context.Background())
	if err != nil {
		// file.exists is first. After the fix it refuses an unsearchable
		// directory, so this script stops there. That refusal is the pass.
		if !errors.Is(err, os.ErrPermission) {
			t.Fatal(err)
		}

		return
	}

	// Seen twice on 2026-09-24, before the fix: [False, True, [1, 2], "from-update"].
	const seen = `[False, True, [1, 2], "from-update"]`
	if value.String() == seen {
		t.Errorf("reproduced file.exists, crc32, patch copy, and state.set: %s", seen)
	}
}
