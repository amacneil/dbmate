package dbmate

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
)

// MigrationOptions is an interface for accessing migration options
type MigrationOptions interface {
	Transaction() bool
}

type migrationOptions map[string]string

// Transaction returns whether or not this migration should run in a transaction
// Defaults to true.
func (m migrationOptions) Transaction() bool {
	return m["transaction"] != "false"
}

// Migration contains the migration contents and options
type Migration struct {
	Contents string
	Options  MigrationOptions
}

// NewMigration constructs a Migration object
func NewMigration() Migration {
	return Migration{Contents: "", Options: make(migrationOptions)}
}

// parseMigration reads a migration file and returns (up Migration, down Migration, error)
func parseMigration(path string) (Migration, Migration, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return NewMigration(), NewMigration(), err
	}
	up, down, err := parseMigrationContents(string(data))
	return up, down, err
}

var upRegExp = regexp.MustCompile(`(?m)^--\s*migrate:up\s+(.+)*$`)
var downRegExp = regexp.MustCompile(`(?m)^--\s*migrate:down\s+(.+)*$`)

// parseMigrationContents parses the string contents of a migration.
// It will return two Migration objects, the first representing the "up"
// block and the second representing the "down" block. This function
// requires that at least an up block was defined and will otherwise
// return an error.
func parseMigrationContents(contents string) (Migration, Migration, error) {
	up := NewMigration()
	down := NewMigration()

	upMatch := upRegExp.FindStringSubmatchIndex(contents)
	downMatch := downRegExp.FindStringSubmatchIndex(contents)

	didNotDefineUpBlock := len(upMatch) == 0
	onlyDefinedUpBlock := len(upMatch) != 0 && len(downMatch) == 0

	if didNotDefineUpBlock {
		return up, down, fmt.Errorf("dbmate requires each migration to define an up bock with '-- migrate:up'")
	} else if onlyDefinedUpBlock {
		up.Contents = strings.TrimSpace(contents)
		up.Options = parseMigrationOptions(contents, upMatch[2], upMatch[3])
	} else {
		upStart := upMatch[0]
		downStart := downMatch[0]

		upEnd := downMatch[0]
		downEnd := len(contents)

		// If migrate:down was defined above migrate:up, correct the end indices
		if upMatch[0] > downMatch[0] {
			upEnd = downEnd
			downEnd = upMatch[0]
		}

		up.Contents = strings.TrimSpace(contents[upStart:upEnd])
		up.Options = parseMigrationOptions(contents, upMatch[2], upMatch[3])

		down.Contents = strings.TrimSpace(contents[downStart:downEnd])
		down.Options = parseMigrationOptions(contents, downMatch[2], downMatch[3])
	}

	return up, down, nil
}

var whitespaceRegExp = regexp.MustCompile(`\s+`)
var optionSeparatorRegExp = regexp.MustCompile(`:`)

// parseMigrationOptions parses the options portion of a migration
// block into an object that satisfies the MigrationOptions interface,
// i.e., the 'transaction:false' piece of the following:
//
//     -- migrate:up transaction:false
//     create table users (id serial, name string);
//     -- migrate:down
//     drop table users;
//
func parseMigrationOptions(contents string, begin, end int) MigrationOptions {
	mOpts := make(migrationOptions)

	if begin == -1 || end == -1 {
		return mOpts
	}

	optionsString := strings.TrimSpace(contents[begin:end])

	optionGroups := whitespaceRegExp.Split(optionsString, -1)
	for _, group := range optionGroups {
		pair := optionSeparatorRegExp.Split(group, -1)
		if len(pair) == 2 {
			mOpts[pair[0]] = pair[1]
		}
	}

	return mOpts
}
