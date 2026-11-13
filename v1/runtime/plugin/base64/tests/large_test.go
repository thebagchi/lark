package tests

import (
	stdbase64 "encoding/base64"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// LARGE_SIZES are data sizes where a kernel runs, in more than one block, and
	// where the last group is whole, two bytes or one byte.
	//
	// Revisions:
	//   - 2026-11-04 19:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LARGE_SIZES = []int{47, 48, 49, 50, 99, 100, 101, 4096, 100000, 100001, 100002, 1 << 20}
)

// _Text is the text of a string or bytes value that a builtin returned.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Text(value starlark.Value) string {
	switch held := value.(type) {
	case starlark.String:
		return string(held)
	case starlark.Bytes:
		return string(held)
	}

	return ""
}

// _Call calls a builtin of the module on one argument and returns the value.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Call(
	t *testing.T,
	name string,
	argument starlark.Value,
) (starlark.Value, error) {
	t.Helper()

	return _Builtin(
		t,
		name,
	).CallInternal(
		&starlark.Thread{
			Name: BENCH,
		},
		starlark.Tuple{
			argument,
		},
		nil,
	)
}

// TestKernels_BuiltinsKeepTheirResultsAtLargeSizes proves the four builtins
// give what encoding/base64 gives, on input that the kernels handle, at sizes
// round their blocks and in whole or part groups at the end.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_BuiltinsKeepTheirResultsAtLargeSizes(t *testing.T) {
	for _, size := range LARGE_SIZES {
		t.Run(
			strconv.Itoa(size),
			func(t *testing.T) {
				data := _Random(
					uint64(size),
					size,
				)

				for _, item := range []struct {
					encode string
					decode string
					text   string
					padded string
				}{
					{
						spec.ENCODE,
						spec.DECODE,
						stdbase64.StdEncoding.EncodeToString(data),
						stdbase64.StdEncoding.EncodeToString(data),
					},
					{
						spec.URLENCODE,
						spec.URLDECODE,
						stdbase64.RawURLEncoding.EncodeToString(data),
						stdbase64.URLEncoding.EncodeToString(data),
					},
				} {
					got, err := _Call(
						t,
						item.encode,
						starlark.Bytes(data),
					)
					if err != nil || _Text(got) != item.text {
						t.Fatalf(
							"%s: wrong text or error %v",
							item.encode,
							err,
						)
					}

					for _, text := range []string{item.text, item.padded} {
						got, err = _Call(
							t,
							item.decode,
							starlark.String(text),
						)
						if err != nil || _Text(got) != string(data) {
							t.Fatalf(
								"%s: wrong bytes or error %v",
								item.decode,
								err,
							)
						}
					}
				}
			},
		)
	}
}

// TestKernels_BuiltinsRefuseWithTheSameText proves that every refusal of C22
// and of the alphabet keeps its text on input that the kernels handle: the text
// the builtin raises is the one that it named before the kernels: the name, the
// quoted prefix of the text, and the sentinel.
//
// Revisions:
//   - 2026-11-03 23:03: initial creation (C51)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestKernels_BuiltinsRefuseWithTheSameText(t *testing.T) {
	const SIZE = 100000

	var (
		data = _Random(
			SIZE,
			SIZE,
		)
		padded = stdbase64.StdEncoding.EncodeToString(data)
		bare   = stdbase64.RawStdEncoding.EncodeToString(data)
		url    = stdbase64.RawURLEncoding.EncodeToString(data)
	)

	// A last character that sets the pad bits: the text ends in two characters,
	// whose last one carries 4 bits that must be zero.
	if len(bare)%4 != 2 {
		t.Fatalf(
			"the test data does not end in a part group: %d",
			len(bare)%4,
		)
	}

	for _, item := range []struct {
		name string
		call string
		text string
	}{
		{
			"bad first",
			spec.DECODE,
			"*" + bare[1:],
		},
		{
			"bad middle",
			spec.DECODE,
			bare[:SIZE/2] + "*" + bare[SIZE/2+1:],
		},
		{
			"bad last",
			spec.DECODE,
			bare[:len(bare)-1] + "*",
		},
		{
			"url char in std",
			spec.DECODE,
			bare[:SIZE/2] + "-" + bare[SIZE/2+1:],
		},
		{
			"std char in url",
			spec.URLDECODE,
			url[:SIZE/2] + "+" + url[SIZE/2+1:],
		},
		{
			"slash in url",
			spec.URLDECODE,
			url[:SIZE/2] + "/" + url[SIZE/2+1:],
		},
		{
			"excess padding",
			spec.DECODE,
			padded + "=",
		},
		{
			"wrong padding",
			spec.DECODE,
			bare + "=",
		},
		{
			"padding after group",
			spec.DECODE,
			bare[:len(bare)-3] + "=",
		},
		{
			"line feed",
			spec.DECODE,
			bare[:SIZE/2] + "\n" + bare[SIZE/2:],
		},
		{
			"carriage return",
			spec.DECODE,
			bare + "\r",
		},
		{
			"one character left",
			spec.DECODE,
			bare[:len(bare)-1],
		},
		{
			"pad bits",
			spec.DECODE,
			bare[:len(bare)-1] + "B",
		},
		{
			"space",
			spec.DECODE,
			bare[:64] + " " + bare[65:],
		},
		{
			"non ASCII",
			spec.DECODE,
			bare[:64] + "é" + bare[66:],
		},
	} {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Call(
					t,
					item.call,
					starlark.String(item.text),
				)

				var want = "base64." + item.call + " got " +
					unpack.QuotePrefix(item.text) + ": " +
					unpack.ErrEncoded.Error()

				if err == nil || err.Error() != want {
					t.Fatalf(
						"got %v, want %q",
						err,
						want,
					)
				}
			},
		)
	}

	if strings.Contains(
		bare,
		"=",
	) {
		t.Fatalf("test data has padding")
	}
}
