package utils

import (
	"io"
	"net/url"
)

// databaseName returns the database name from a URL
func DatabaseName(u *url.URL) string {
	name := u.Path
	if len(name) > 0 && name[:1] == "/" {
		name = name[1:len(name)]
	}

	return name
}

func MustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		panic(err)
	}
}
