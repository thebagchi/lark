// Package spec is internal, not public: the names the hash module and its
// arguments spell, and the checksum seed.
//
// Revisions:
//   - 2026-11-08 00:26: initial creation (C91w12b)
package spec

// NAME is the module, and the names it holds.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:26: moved here from plugin/hash (C91w12b)
const (
	NAME   = "hash"
	MD5    = "md5"
	SHA1   = "sha1"
	SHA256 = "sha256"
	SHA512 = "sha512"
	HMAC   = "hmac"
	CRC32  = "crc32"

	// KEY and DATA are what hmac calls its arguments, so a script may pass
	// them either way round, and ALGORITHM which digest to key.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/hash (C91w12b)
	ALGORITHM = "algorithm"
	KEY       = "key"
	DATA      = "data"

	// NO_SEED is the checksum a chain starts from.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 00:26: moved here from plugin/hash (C91w12b)
	NO_SEED = 0
)
