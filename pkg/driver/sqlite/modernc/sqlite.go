package sqlite

import (
	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/driver/sqlite/internal"

	_ "modernc.org/sqlite" // database/sql driver
)

func init() {
	dbmate.RegisterDriver(internal.NewDriver("sqlite"), "sqlite")
	dbmate.RegisterDriver(internal.NewDriver("sqlite"), "sqlite3")
}
