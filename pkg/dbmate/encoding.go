package dbmate

import "strings"

// detectUnsupportedEncoding describes the encoding of a migration file that
// looks like UTF-16 or UTF-32, or returns "" if it does not. It is only used
// to explain a parse error, so it never causes a migration to be rejected.
//
// Without a byte order mark, the guess relies on every SQL file starting with
// an ASCII character, even if only whitespace. In UTF-16 and UTF-32 the other
// bytes of that character are zero, while UTF-8 never starts with a zero byte.
func detectUnsupportedEncoding(contents string) string {
	isASCII := func(c byte) bool { return c != 0 && c < 0x80 }

	switch {
	// UTF-32 is checked first, because its little-endian byte order mark
	// begins with the UTF-16LE one
	case strings.HasPrefix(contents, "\xff\xfe\x00\x00"):
		return "UTF-32LE with a byte order mark"
	case strings.HasPrefix(contents, "\x00\x00\xfe\xff"):
		return "UTF-32BE with a byte order mark"
	case strings.HasPrefix(contents, "\xff\xfe"):
		return "UTF-16LE with a byte order mark"
	case strings.HasPrefix(contents, "\xfe\xff"):
		return "UTF-16BE with a byte order mark"
	case len(contents) >= 4 && isASCII(contents[0]) && contents[1:4] == "\x00\x00\x00":
		return "UTF-32LE without a byte order mark"
	case len(contents) >= 4 && contents[0:3] == "\x00\x00\x00" && isASCII(contents[3]):
		return "UTF-32BE without a byte order mark"
	case len(contents) >= 2 && isASCII(contents[0]) && contents[1] == 0:
		return "UTF-16LE without a byte order mark"
	case len(contents) >= 2 && contents[0] == 0 && isASCII(contents[1]):
		return "UTF-16BE without a byte order mark"
	default:
		return ""
	}
}
