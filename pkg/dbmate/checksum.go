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

// ParseChecksumMode parses environment/CLI strings to a ChecksumMode.
// Accepted strings (case-insensitive): "NONE", "LENIENT", "STRICT".
func ParseChecksumMode(s string) (ChecksumMode, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "NONE":
		return ChecksumNone, nil
	case "LENIENT":
		return ChecksumLenient, nil
	case "STRICT":
		return ChecksumStrict, nil
	default:
		return ChecksumNone, ErrUnknownChecksumMode
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

// ComputeChecksum returns the hex SHA-256 of the supplied bytes.
// It is platform resilient, normalizing CRLF to LF
func ComputeChecksum(b []byte) string {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
