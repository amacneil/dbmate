package dbmate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

type ChecksumMode int

const (
	ChecksumNone ChecksumMode = iota
	ChecksumLenient
	ChecksumStrict
)

var ErrUnknownChecksumMode = errors.New("unknown checksum mode")

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ParseChecksumMode parses environment/CLI strings to a ChecksumMode.
// Accepted strings (case-insensitive): "NONE", "LENIENT", "STRICT".
func ParseChecksumMode(s string) (ChecksumMode, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "NONE":
		return ChecksumNone, nil
	case "", "LENIENT":
		return ChecksumLenient, nil
	case "STRICT":
		return ChecksumStrict, nil
	default:
		return ChecksumLenient, ErrUnknownChecksumMode
	}
}

func ModeToString(m ChecksumMode) string {
	switch m {
	case ChecksumNone:
		return "NONE"
	case ChecksumLenient:
		return "LENIENT"
	case ChecksumStrict:
		return "STRICT"
	default:
		return "UNKNOWN"
	}
}

// ComputeChecksum computes a SHA256 checksum of the given bytes after
// canonicalizing text. We strip a leading UTF-8 BOM (if present) and normalize
// CRLF -> LF so checksums are stable across platforms.
func ComputeChecksum(b []byte) string {
	// strip UTF-8 BOM if present
	b = bytes.TrimPrefix(b, utf8BOM)

	// normalize CRLF -> LF
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
