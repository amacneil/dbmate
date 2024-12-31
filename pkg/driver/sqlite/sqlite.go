//go:build cgo
// +build cgo

package sqlite

// use the "github.com/mattn/go-sqlite3" driver as a default
import _ "github.com/amacneil/dbmate/v2/pkg/driver/sqlite/mattn"
