// +build go1.16

package dbmate

import (
	"io/fs"
	"net/url"
	"os"
	"path"
	"time"
)

// DB allows dbmate actions to be performed on a specified database
type DB struct {
	AutoDumpSchema      bool
	DatabaseURL         *url.URL
	MigrationsDir       string
	MigrationsTableName string
	SchemaFile          string
	Verbose             bool
	WaitBefore          bool
	WaitInterval        time.Duration
	WaitTimeout         time.Duration
	FS                  fs.FS
}

// New initializes a new dbmate database
func New(databaseURL *url.URL) *DB {
	return &DB{
		AutoDumpSchema:      true,
		DatabaseURL:         databaseURL,
		MigrationsDir:       DefaultMigrationsDir,
		MigrationsTableName: DefaultMigrationsTableName,
		SchemaFile:          DefaultSchemaFile,
		WaitBefore:          false,
		WaitInterval:        DefaultWaitInterval,
		WaitTimeout:         DefaultWaitTimeout,
		FS:                  osFS{},
	}
}

// osFS is an fs.FS implementation that just passes on to os.Open
type osFS struct{}

func (c osFS) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func (db *DB) readDir(name string) ([]fs.DirEntry, error) {
	// Clean the path to remove any leading ./
	name = path.Clean(name)
	return fs.ReadDir(db.FS, name)
}

func (db *DB) readFile(name string) ([]byte, error) {
	// Clean the path to remove leading ./
	name = path.Clean(name)
	return fs.ReadFile(db.FS, name)
}
