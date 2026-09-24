package testing_test

import (
	"context"
	"errors"
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

func TestDuration_PastInt64IsRefused(t *testing.T) {
	seconds := float64(uint64(math.MaxInt64)+1) / float64(time.Second)
	_, err := scheduler.Duration(starlark.Float(seconds))
	if !errors.Is(err, scheduler.ErrDuration) {
		t.Fatalf("got %v, want ErrDuration", err)
	}
}

func TestExists_BehindAClosedDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can search a mode 000 directory")
	}

	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(secret, "note")
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	_, err := _Run(t, `
def main():
    file.exists("`+note+`")
    return "reached"
`)
	if err == nil {
		t.Fatal("exists on an unsearchable path returned instead of refusing")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("got %v, want a permission refusal", err)
	}
}

func TestScript_CRCAndCopyAndSet(t *testing.T) {
	value, err := _Run(t, `
def main():
    wide = "refused"
    caught = ""
    # A failing builtin is a failing script, so the wide seed is its own run.

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
    return [doc["a"], state.get("n"), crc32(b"abc", 4294967295) != crc32(b"abc", 0)]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `[[1], "from-set", True]` {
		t.Fatalf("got %s", value.String())
	}

	_, err = _Run(t, `
def main():
    return crc32(b"abc", 4294967296)
`)
	if err == nil {
		t.Fatal("a 2**32 seed was accepted")
	}
}

func _Run(t *testing.T, src string) (starlark.Value, error) {
	t.Helper()

	built, err := runtime.NewCompiler().Compile("fixed.star", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	return built.Run(context.Background())
}
