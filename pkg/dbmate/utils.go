package dbmate

import (
	"bufio"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// databaseName returns the database name from a URL
func databaseName(u *url.URL) string {
	name := u.Path
	if len(name) > 0 && name[:1] == "/" {
		name = name[1:]
	}

	return name
}

// mustClose ensures a stream is closed
func mustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
}

// ensureDir creates a directory if it does not already exist
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("unable to create directory `%s`", dir)
	}

	return nil
}

// runCommand runs a command and returns the stdout if successful
func runCommand(name string, args ...string) ([]byte, error) {
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

// trimLeadingSQLComments removes sql comments and blank lines from the beginning of text
// generally when performing sql dumps these contain host-specific information such as
// client/server version numbers
func trimLeadingSQLComments(data []byte) ([]byte, error) {
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

// queryColumn runs a SQL statement and returns a slice of strings
// it is assumed that the statement returns only one column
// e.g. schema_migrations table
func queryColumn(db *sql.DB, query string) ([]string, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer mustClose(rows)

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
