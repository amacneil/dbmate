package sqlite

import (
	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite/internal"

	_ "github.com/mattn/go-sqlite3" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(internal.NewDriver("sqlite3"), "sqlite")
	dbmate.RegisterDriver(internal.NewDriver("sqlite3"), "sqlite3")
}
