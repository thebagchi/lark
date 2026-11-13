package buffer

// NAME is the module, and the two names it holds.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved from package buf (C91w8a)
const (
	NAME   = "buf"
	NEW    = "new"
	READER = "reader"

	// WRITE_BYTES to BYTES are the methods of a buffer.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	WRITE_BYTES  = "write_bytes"
	WRITE_STRING = "write_string"
	WRITE_UINT   = "write_uint"
	WRITE_INT    = "write_int"
	WRITE_UINT8  = "write_uint8"
	WRITE_UINT16 = "write_uint16"
	WRITE_UINT32 = "write_uint32"
	WRITE_UINT64 = "write_uint64"
	WRITE_INT8   = "write_int8"
	WRITE_INT16  = "write_int16"
	WRITE_INT32  = "write_int32"
	WRITE_INT64  = "write_int64"
	BYTES        = "bytes"

	// READ_BYTES to READ_INT64 are the methods of a reader.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	READ_BYTES  = "read_bytes"
	READ_STRING = "read_string"
	READ_UINT   = "read_uint"
	READ_INT    = "read_int"
	READ_UINT8  = "read_uint8"
	READ_UINT16 = "read_uint16"
	READ_UINT32 = "read_uint32"
	READ_UINT64 = "read_uint64"
	READ_INT8   = "read_int8"
	READ_INT16  = "read_int16"
	READ_INT32  = "read_int32"
	READ_INT64  = "read_int64"

	// WIDTH_8 to WIDTH_64 are how many bytes an integer of 8 to 64 bits takes.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	WIDTH_8  = 1
	WIDTH_16 = 2
	WIDTH_32 = 4
	WIDTH_64 = 8

	// BUFFER_TYPE and READER_TYPE are what type() calls the two values.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	BUFFER_TYPE = "buffer"
	READER_TYPE = "reader"
)
