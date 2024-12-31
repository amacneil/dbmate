package sqlite

import (
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite/internal"

	_ "modernc.org/sqlite" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(NewDriver, "sqlite")
	dbmate.RegisterDriver(NewDriver, "sqlite3")
}

// NewDriver initializes the driver
func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return internal.NewDriver("sqlite")(config)
}

// ConnectionString converts a URL into a valid connection string
func ConnectionString(u *url.URL) string {
	return internal.ConnectionString(u)
}
