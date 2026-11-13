package host

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// TEMP_DIR is the mode of the private directory ListenTemp makes: this
	// user alone, so no other user can reach the socket inside it.
	//
	// Revisions:
	//   - 2026-11-08 02:10: initial creation (C84, Q240)
	TEMP_DIR = 0o700

	// TEMP_PREFIX starts the name of that directory.
	//
	// Revisions:
	//   - 2026-11-08 02:10: initial creation (C84, Q240)
	TEMP_PREFIX = "lark-"

	// TEMP_SOCKET is the file name of the socket in that directory. It is
	// short because a unix socket path has a low length limit.
	//
	// Revisions:
	//   - 2026-11-08 02:10: initial creation (C84, Q240)
	TEMP_SOCKET = "plugins.sock"

	// TOKEN_BYTES is the size of the random secret: 16 bytes, which is 128
	// bits, from crypto/rand.
	//
	// Revisions:
	//   - 2026-11-08 02:10: initial creation (C84, Q240)
	TOKEN_BYTES = 16
)

// ListenTemp serves on a socket in a new private directory, with a new random
// token.
//
// The directory is made with TEMP_DIR. Close on the returned Listener removes
// the directory and the socket. A failure here removes the directory before it
// returns. No error holds the token. Never panics.
//
// Revisions:
//   - 2026-11-08 02:10: initial creation (C84, Q240)
//   - 2026-11-10 16:05: cuts the directory path and the path in its error to common.QUOTE_LIMIT
//     bytes (C117, B239, Q4)
func ListenTemp() (*Listener, error) {
	var (
		secret = make(
			[]byte,
			TOKEN_BYTES,
		)
		_, err = rand.Read(secret)
	)
	if err != nil {
		return nil, fmt.Errorf(
			"making a token: %w",
			err,
		)
	}

	dir, err := os.MkdirTemp(
		"",
		TEMP_PREFIX,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"making a private directory: %w",
			err,
		)
	}

	err = os.Chmod(
		dir,
		TEMP_DIR,
	)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf(
				"securing %s: %w",
				unpack.ShortText(dir),
				unpack.ShortPath(err),
			),
			os.RemoveAll(dir),
		)
	}

	held, err := Listen(
		filepath.Join(
			dir,
			TEMP_SOCKET,
		),
		hex.EncodeToString(secret),
	)
	if err != nil {
		return nil, errors.Join(
			err,
			os.RemoveAll(dir),
		)
	}

	held.cleanup = dir

	return held, nil
}
