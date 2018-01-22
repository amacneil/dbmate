package dbmate

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os/exec"
	"strings"
)

// databaseName returns the database name from a URL
func databaseName(u *url.URL) string {
	name := u.Path
	if len(name) > 0 && name[:1] == "/" {
		name = name[1:]
	}

	return name
}

func mustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
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
