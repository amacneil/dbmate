package dbutil

import (
	"bufio"
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"unicode"
)

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// DatabaseName returns the database name from a URL
func DatabaseName(u *url.URL) string {
	name := u.Path
	if len(name) > 0 && name[:1] == "/" {
		name = name[1:]
	}

	return name
}

// MustClose ensures a stream is closed
func MustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
}

// RunCommand runs a command and returns the stdout if successful
func RunCommand(name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// return stderr if available
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return nil, errors.New(s)
		}

		// otherwise return error
		return nil, err
	}

	// return stdout
	return stdout.Bytes(), nil
}

// TrimLeadingSQLComments removes sql comments and blank lines from the beginning of text
// generally when performing sql dumps these contain host-specific information such as
// client/server version numbers
func TrimLeadingSQLComments(data []byte) ([]byte, error) {
	// create decent size buffer
	out := bytes.NewBuffer(make([]byte, 0, len(data)))

	// iterate over sql lines
	preamble := true
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		// we read bytes directly for premature performance optimization
		line := scanner.Bytes()

		if preamble && (len(line) == 0 || bytes.Equal(line[0:2], []byte("--"))) {
			// header section, skip this line in output buffer
			continue
		}

		// header section is over
		preamble = false

		// trim trailing whitespace
		line = bytes.TrimRightFunc(line, unicode.IsSpace)

		// copy bytes to output buffer
		if _, err := out.Write(line); err != nil {
			return nil, err
		}
		if _, err := out.WriteString("\n"); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// QueryColumn runs a SQL statement and returns a slice of strings
// it is assumed that the statement returns only one column
// e.g. schema_migrations table
func QueryColumn(db Transaction, query string, args ...interface{}) ([]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer MustClose(rows)

	// read into slice
	var result []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}

		result = append(result, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// QueryValue runs a SQL statement and returns a single string
// it is assumed that the statement returns only one row and one column
// sql NULL is returned as empty string
func QueryValue(db Transaction, query string, args ...interface{}) (string, error) {
	var result sql.NullString
	err := db.QueryRow(query, args...).Scan(&result)
	if err != nil || !result.Valid {
		return "", err
	}

	return result.String, nil
}

// MustUnescapePath unescapes a URL path, and panics if it fails.
// It is used during in cases where we are parsing a generated path.
func MustUnescapePath(s string) string {
	if s == "" {
		panic("missing path")
	}

	path, err := url.PathUnescape(s)
	if err != nil {
		panic(err)
	}

	return path
}
