package shared

import (
	"database/sql"
	"net/url"
)

// DatabaseName returns the database name from a URL
func DatabaseName(u *url.URL) string {
	name := u.Path
	if len(name) > 0 && name[:1] == "/" {
		name = name[1:len(name)]
	}

	return name
}

// Transaction can represent a database or open transaction
type Transaction interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}
