package tests

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	stdhash "hash"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/spec"
)

const (
	// DIGEST_BYTES is the size of the data the allocation test hashes.
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DIGEST_BYTES = 1024

	// DIGEST_ALLOCS is what one digest call may allocate: the text, and the
	// box that turns the text into a starlark.Value.
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DIGEST_ALLOCS = 2

	// DIGEST_RUNS is how many calls AllocsPerRun averages.
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	DIGEST_RUNS = 100
)

var (
	// MAKERS is the streaming constructor of each digest the module offers:
	// the form the one-shot sums replace, and so the reference they must match.
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MAKERS = map[string]func() stdhash.Hash{
		spec.MD5:    md5.New,
		spec.SHA1:   sha1.New,
		spec.SHA256: sha256.New,
		spec.SHA512: sha512.New,
	}

	// LENGTHS are the data sizes the agreement test tries. They cross the
	// block sizes of the four digests (64 and 128 bytes) and the padding
	// edges at 55, 56, 63, 64, 111, 112, 127 and 128.
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LENGTHS = []int{
		0, 1, 2, 3, 15, 16, 17, 31, 55, 56, 57, 63, 64, 65,
		111, 112, 113, 127, 128, 129, 255, 256, 1000, 1024, 4096, 65536, 1 << 20,
	}

	// VECTORS are digests that the Go standard library test files publish, in
	// the lower case they print. Each name is the file and its table:
	//   crypto/md5/md5_test.go    golden, lines 26 and 29
	//   crypto/sha1/sha1_test.go  golden, lines 28 and 31
	//   crypto/sha256/sha256_test.go  golden, lines 26 and 29
	//   crypto/sha512/sha512_test.go  golden512, lines 502 and 517
	//
	// Revisions:
	//   - 2026-11-03 23:44: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	VECTORS = []struct {
		name  string
		input string
		want  string
	}{
		{
			spec.MD5,
			"",
			"d41d8cd98f00b204e9800998ecf8427e",
		},
		{
			spec.MD5,
			"abc",
			"900150983cd24fb0d6963f7d28e17f72",
		},
		{
			spec.SHA1,
			"",
			"da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			spec.SHA1,
			"abc",
			"a9993e364706816aba3e25717850c26c9cd0d89d",
		},
		{
			spec.SHA256,
			"",
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			spec.SHA256,
			"abc",
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			spec.SHA512,
			"",
			"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce" +
				"47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			spec.SHA512,
			"abc",
			"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
				"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
	}
)

// _Call calls one digest builtin on data, as a script's call reaches it, and
// returns the text it gives back.
//
// Revisions:
//   - 2026-11-03 23:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Call(
	tb testing.TB,
	fn *starlark.Builtin,
	data []byte,
) string {
	tb.Helper()

	value, err := fn.CallInternal(
		new(starlark.Thread),
		starlark.Tuple{
			starlark.Bytes(data),
		},
		nil,
	)
	if err != nil {
		tb.Fatalf(
			"%s: %v",
			fn.Name(),
			err,
		)
	}

	text, ok := value.(starlark.String)
	if !ok {
		tb.Fatalf(
			"%s gave %T, want a string",
			fn.Name(),
			value,
		)
	}

	return string(text)
}

// TestDigest_MakesTwoAllocations proves that a digest of 1 KiB allocates the
// text and the box around it, and nothing else, for each of the four digests.
// Before the one-shot sums the streaming form made four for sha256.
// Run it alone: go test -run TestDigest_MakesTwoAllocations ./v1/runtime/plugin/hash
//
// Revisions:
//   - 2026-11-03 23:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDigest_MakesTwoAllocations(t *testing.T) {
	data := _Data(DIGEST_BYTES)

	for name := range MAKERS {
		t.Run(
			name,
			func(t *testing.T) {
				fn := _Builtin(
					t,
					name,
				)

				var (
					args = starlark.Tuple{
						starlark.Bytes(data),
					}
					thread = new(starlark.Thread)
				)

				got := testing.AllocsPerRun(
					DIGEST_RUNS,
					func() {
						_, err := fn.CallInternal(
							thread,
							args,
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}
					},
				)
				if got != DIGEST_ALLOCS {
					t.Errorf(
						"%s of %d bytes: %v allocations, want %d",
						name,
						DIGEST_BYTES,
						got,
						DIGEST_ALLOCS,
					)
				}
			},
		)
	}
}

// TestDigest_AgreesWithTheStreamingForm proves that the builtin gives the
// bytes the streaming digest gives, as upper-case hex, for every algorithm and
// for lengths from empty to 1 MiB.
//
// Revisions:
//   - 2026-11-03 23:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDigest_AgreesWithTheStreamingForm(t *testing.T) {
	for name, maker := range MAKERS {
		for _, size := range LENGTHS {
			t.Run(
				name+"/"+strconv.Itoa(size),
				func(t *testing.T) {
					_Agree(
						t,
						name,
						maker,
						size,
					)
				},
			)
		}
	}
}

// _Agree checks one algorithm on size bytes against its streaming digest.
//
// Revisions:
//   - 2026-11-03 23:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Agree(
	t *testing.T,
	name string,
	maker func() stdhash.Hash,
	size int,
) {
	t.Helper()

	var (
		data      = _Data(size)
		streaming = maker()
	)

	_, err := streaming.Write(data)
	if err != nil {
		t.Fatalf(
			"write: %v",
			err,
		)
	}

	var (
		sum  = streaming.Sum(nil)
		want = strings.ToUpper(hex.EncodeToString(sum))
		got  = _Call(
			t,
			_Builtin(
				t,
				name,
			),
			data,
		)
	)
	if got != want {
		t.Errorf(
			"got %s, want %s",
			got,
			want,
		)
	}
}

// TestDigest_GivesThePublishedVectors proves the builtin against digests the Go
// standard library publishes, so the check does not rest on the streaming form
// alone. It also proves the call leaves its input as it found it.
//
// Revisions:
//   - 2026-11-03 23:05: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDigest_GivesThePublishedVectors(t *testing.T) {
	for _, vector := range VECTORS {
		t.Run(
			vector.name+"/"+strconv.Quote(vector.input),
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						vector.name,
					)
					input = []byte(vector.input)
					want  = strings.ToUpper(vector.want)
					got   = _Call(
						t,
						fn,
						input,
					)
				)
				if got != want {
					t.Errorf(
						"got %s, want %s",
						got,
						want,
					)
				}

				if !bytes.Equal(
					input,
					[]byte(vector.input),
				) {
					t.Errorf("the call changed its input")
				}
			},
		)
	}
}
