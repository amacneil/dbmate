package dbmate

import (
	"errors"
	"testing"
	"testing/fstest"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
)

// encodeUTF16 encodes s as UTF-16 in the given byte order, without a byte
// order mark.
func encodeUTF16(s string, bigEndian bool) string {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		if bigEndian {
			b = append(b, byte(u>>8), byte(u))
		} else {
			b = append(b, byte(u), byte(u>>8))
		}
	}
	return string(b)
}

// encodeUTF32 encodes s as UTF-32 in the given byte order, without a byte
// order mark.
func encodeUTF32(s string, bigEndian bool) string {
	var b []byte
	for _, r := range s {
		if bigEndian {
			b = append(b, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
		} else {
			b = append(b, byte(r), byte(r>>8), byte(r>>16), byte(r>>24))
		}
	}
	return string(b)
}

const encodingTestMigration = "-- migrate:up\ninsert into t values ('東京 😀');\n-- migrate:down\ndelete from t;\n"

func parseMigrationFile(t *testing.T, data string) ([]*ParsedMigration, error) {
	t.Helper()
	migration := &Migration{
		FileName: "123_foo.sql",
		FilePath: "123_foo.sql",
		FS:       fstest.MapFS{"123_foo.sql": {Data: []byte(data)}},
		Version:  "123",
	}
	return migration.Parse()
}

func TestDetectUnsupportedEncoding(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"UTF-32LE with BOM", "\xff\xfe\x00\x00" + encodeUTF32("-- migrate:up\n", false), "UTF-32LE with a byte order mark"},
		{"UTF-32BE with BOM", "\x00\x00\xfe\xff" + encodeUTF32("-- migrate:up\n", true), "UTF-32BE with a byte order mark"},
		{"UTF-16LE with BOM", "\xff\xfe" + encodeUTF16("-- migrate:up\n", false), "UTF-16LE with a byte order mark"},
		{"UTF-16BE with BOM", "\xfe\xff" + encodeUTF16("-- migrate:up\n", true), "UTF-16BE with a byte order mark"},
		{"UTF-32LE without BOM", encodeUTF32("-- migrate:up\n", false), "UTF-32LE without a byte order mark"},
		{"UTF-32BE without BOM", encodeUTF32("-- migrate:up\n", true), "UTF-32BE without a byte order mark"},
		{"UTF-16LE without BOM", encodeUTF16("-- migrate:up\n", false), "UTF-16LE without a byte order mark"},
		{"UTF-16BE without BOM", encodeUTF16("-- migrate:up\n", true), "UTF-16BE without a byte order mark"},
		{"UTF-16LE without BOM, leading blank line", encodeUTF16("\n-- migrate:up\n", false), "UTF-16LE without a byte order mark"},
		{"UTF-16BE without BOM, leading CRLF and indent", encodeUTF16("\r\n  -- comment\n", true), "UTF-16BE without a byte order mark"},
		{"UTF-8", "-- migrate:up\n", ""},
		{"UTF-8 with BOM", "\xef\xbb\xbf-- migrate:up\n", ""},
		{"Latin-1", "-- caf\xe9\n", ""},
		{"empty", "", ""},
		{"single byte", "-", ""},
		{"zero bytes", "\x00\x00\x00\x00", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, detectUnsupportedEncoding(c.input))
		})
	}
}

func TestParseUnsupportedEncoding(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		encoding string
	}{
		{"UTF-16LE with BOM", "\xff\xfe" + encodeUTF16(encodingTestMigration, false), "UTF-16LE with a byte order mark"},
		{"UTF-16BE with BOM", "\xfe\xff" + encodeUTF16(encodingTestMigration, true), "UTF-16BE with a byte order mark"},
		{"UTF-32LE with BOM", "\xff\xfe\x00\x00" + encodeUTF32(encodingTestMigration, false), "UTF-32LE with a byte order mark"},
		{"UTF-32BE with BOM", "\x00\x00\xfe\xff" + encodeUTF32(encodingTestMigration, true), "UTF-32BE with a byte order mark"},
		{"UTF-16LE without BOM", encodeUTF16(encodingTestMigration, false), "UTF-16LE without a byte order mark"},
		{"UTF-16BE without BOM, leading whitespace", encodeUTF16("\r\n  "+encodingTestMigration, true), "UTF-16BE without a byte order mark"},
		{"UTF-32LE without BOM", encodeUTF32(encodingTestMigration, false), "UTF-32LE without a byte order mark"},
		{"UTF-32BE without BOM", encodeUTF32(encodingTestMigration, true), "UTF-32BE without a byte order mark"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseMigrationFile(t, c.input)
			require.ErrorIs(t, err, ErrParseMissingUp)
			require.EqualError(t, err, ErrParseMissingUp.Error()+" (file appears to be "+c.encoding+", convert it to UTF-8)")
		})
	}

	t.Run("UTF-8 parse error has no encoding hint", func(t *testing.T) {
		_, err := parseMigrationFile(t, "-- migrate:up\ncreate table t (id int);\n")
		require.ErrorIs(t, err, ErrParseMissingDown)
		require.EqualError(t, err, ErrParseMissingDown.Error())
	})

	t.Run("non-UTF-8 bytes reach the parsed SQL unchanged", func(t *testing.T) {
		parsed, err := parseMigrationFile(t, "-- migrate:up\ninsert into t values ('caf\xe9');\n-- migrate:down\n")
		require.NoError(t, err)
		require.Equal(t, "-- migrate:up\ninsert into t values ('caf\xe9');\n", parsed[0].Up)
	})
}

// FuzzParseEncodingHint checks that the encoding hint only ever changes the
// text of an error: a file parses through Migration.Parse exactly when it
// parses through parseMigrationContents, with the same result.
func FuzzParseEncodingHint(f *testing.F) {
	f.Add(encodingTestMigration)
	f.Add("\xef\xbb\xbf" + encodingTestMigration)
	f.Add("\xff\xfe" + encodeUTF16(encodingTestMigration, false))
	f.Add(encodeUTF16(encodingTestMigration, true))

	f.Fuzz(func(t *testing.T, contents string) {
		expected, expectedErr := parseMigrationContents(contents)
		parsed, err := parseMigrationFile(t, contents)

		require.Equal(t, expected, parsed)
		if expectedErr == nil {
			require.NoError(t, err)
			return
		}
		require.True(t, errors.Is(err, expectedErr), "error %q does not wrap %q", err, expectedErr)
	})
}
