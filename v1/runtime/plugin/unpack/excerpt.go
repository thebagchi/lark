package unpack

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// QuotePrefix is text as %q writes it, when text has QUOTE_LIMIT bytes or fewer. Longer
// text is cut to at most QUOTE_LIMIT bytes, quoted, and followed by a marker outside
// the closing quote that holds the whole length in bytes:
//
//	"abc"... (5000000 bytes)
//
// The cut comes before the quote. The work and the result then do not grow
// with the text, and no escape is ever split, because the escapes are made
// after the cut. A quoted byte takes at most four bytes (\xNN), so the quoted
// part is at most 4*QUOTE_LIMIT+2 bytes.
//
// Every error message that holds text a script supplied calls QuotePrefix, so one
// path cannot forget the bound. Advisory GHSA-7hhh-6rmp-j9qf (jackson-core, alias
// CVE-2026-89425, https://api.osv.dev/v1/vulns/GHSA-7hhh-6rmp-j9qf) is the
// incident when a path forgets it: a 20 MB token gave a 20 MB message.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation, for decision Q4 (change C21)
//   - 2026-11-02 11:19: renamed from Quote
//   - 2026-11-02 11:30: cite the advisory by id and URL
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func QuotePrefix(text string) string {
	if len(text) <= common.QUOTE_LIMIT {
		return strconv.Quote(text)
	}

	return strconv.Quote(text[:_Cut(text)]) + _Marker(text)
}

// ShortText is text as it is, when text has QUOTE_LIMIT bytes or fewer. Longer text is
// cut to at most QUOTE_LIMIT bytes and followed by the same marker as QuotePrefix writes.
//
// Use ShortText where today's message prints the text with no quote, such as the
// digits of an integer, and the short text must keep its bytes. Use QuotePrefix for
// anything that can hold a line break or a control byte.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation, for decision Q4 (change C21)
//   - 2026-11-02 11:19: renamed from Short
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func ShortText(text string) string {
	if len(text) <= common.QUOTE_LIMIT {
		return text
	}

	return text[:_Cut(text)] + _Marker(text)
}

// ShortPath is cause with the file name it carries cut as ShortText cuts text.
//
// The operating system refuses a name that is too long, and Go still keeps the
// whole name in the error: a 1,000,000-byte name gave an error of 1,000,025
// bytes. The copy keeps Op and Err, so errors.Is and errors.As find what they
// found, and the text keeps its shape. A name of QUOTE_LIMIT bytes or fewer gives
// the same text. Only an error that is itself a *fs.PathError, an
// *os.LinkError or a *net.OpError that names a unix socket is copied. The last
// is cut because listening on a path too long for the system gives an error
// that holds the whole path (B269). An error that wraps one, and any other
// error, comes back as it is, because the copy would lose the words of the
// wrapper.
//
// Revisions:
//   - 2026-11-02 11:02: initial creation, moved from the file plugin so that the loader
//     of a script's modules cuts a name the same way (change C21)
//   - 2026-11-02 11:19: renamed from Path
//   - 2026-11-11 00:42: cuts the address of a unix socket in a *net.OpError (C122, B269)
func ShortPath(cause error) error {
	var (
		path   *fs.PathError
		link   *os.LinkError
		socket *net.OpError
	)

	switch {
	case errors.As(
		cause,
		&path,
	) && path.Error() == cause.Error():
		return &fs.PathError{
			Op:   path.Op,
			Path: ShortText(path.Path),
			Err:  path.Err,
		}

	case errors.As(
		cause,
		&link,
	) && link.Error() == cause.Error():
		return &os.LinkError{
			Op:  link.Op,
			Old: ShortText(link.Old),
			New: ShortText(link.New),
			Err: link.Err,
		}

	case errors.As(
		cause,
		&socket,
	) && socket.Error() == cause.Error():
		return _Shortened(socket)

	default:
		return cause
	}
}

// _Shortened is op with the unix address it names cut as ShortText cuts text.
// An error of any other network, and one with no address, comes back as it is.
//
// Revisions:
//   - 2026-11-11 00:42: initial creation (C122, B269)
func _Shortened(op *net.OpError) error {
	addr, ok := op.Addr.(*net.UnixAddr)
	if !ok || addr == nil {
		return op
	}

	return &net.OpError{
		Op:     op.Op,
		Net:    op.Net,
		Source: op.Source,
		Addr: &net.UnixAddr{
			Name: ShortText(addr.Name),
			Net:  addr.Net,
		},
		Err: op.Err,
	}
}

// _Marker is the text after a cut: the dots and the whole length in bytes.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
func _Marker(text string) string {
	return "... (" + strconv.Itoa(len(text)) + " bytes)"
}

// _Cut is the largest index at most QUOTE_LIMIT where text[:index] does not end in
// the middle of a valid multibyte rune. It looks back at most utf8.UTFMax-1
// bytes, so a run of stray continuation bytes costs three steps, not a scan,
// and keeps its bytes. Go's TruncatePointer backs off the same way.
//
// Revisions:
//   - 2026-11-02 10:55: initial creation
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func _Cut(text string) int {
	for back := 0; back < utf8.UTFMax; back++ {
		start := common.QUOTE_LIMIT - back
		if !utf8.RuneStart(text[start]) {
			continue
		}

		_, width := utf8.DecodeRuneInString(text[start:])
		if start+width > common.QUOTE_LIMIT && width > 1 {
			return start
		}

		return common.QUOTE_LIMIT
	}

	return common.QUOTE_LIMIT
}
