// +build !go1.16

package dbmate

import (
	"io/ioutil"
	"net/url"
	"os"
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
	}
}

func (db *DB) readDir(path string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(path)
}

func (db *DB) readFile(path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}
