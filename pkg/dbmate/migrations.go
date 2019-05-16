package dbmate

import (
	"io/ioutil"
	"regexp"
	"strings"
)

// MigrateOptions is an interface for accessing migration options
type MigrateOptions interface {
	SkipTransaction() bool
}

type migrateOptions map[string]string

// SkipTransaction returns true if the migration is to run outside a transaction
// Defaults to false.
func (m migrateOptions) SkipTransaction() bool {
	return m["skip_transaction"] == "true"
}

// Migrate contains 'up' or 'down' migration commands and options
type Migrate struct {
	Direction string
	Contents  string
	Options   MigrateOptions
}

// NewMigrate constructs a Migrate object
func NewMigrate(direction string) Migrate {
	return Migrate{
		Direction: direction,
		Contents:  "",
		Options:   make(migrateOptions),
	}
}

// parseMigration reads a migration file and returns (up Migrate, down Migrate, error)
func parseMigration(path string) (Migrate, Migrate, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return NewMigrate("up"), NewMigrate("down"), err
	}
	up, down := parseMigrationContents(string(data))
	return up, down, nil
}

var upRegExp = regexp.MustCompile(`(?m)^-- migrate:up\s+(.+)*$`)
var downRegExp = regexp.MustCompile(`(?m)^-- migrate:down\s+(.+)*$`)

// parseMigrationContents parses the string contents of a migration.
// It will return two Migrate objects, the first representing the "up"
// block and the second representing the "down" block.
//
// Note that with the way this is currently defined, it is possible to
// correctly parse a migration that does not define an "up" block or a
// "down" block, or one that defines neither. This behavior is, in part,
// to preserve backwards compatibility.
func parseMigrationContents(contents string) (Migrate, Migrate) {
	up := NewMigrate("up")
	down := NewMigrate("down")

	upMatch := upRegExp.FindStringSubmatchIndex(contents)
	downMatch := downRegExp.FindStringSubmatchIndex(contents)

	onlyDefinedUpBlock := len(upMatch) != 0 && len(downMatch) == 0
	onlyDefinedDownBlock := len(upMatch) == 0 && len(downMatch) != 0

	if onlyDefinedUpBlock {
		up.Contents = strings.TrimSpace(contents)
		up.Options = parseMigrateOptions(contents, upMatch[2], upMatch[3])
	} else if onlyDefinedDownBlock {
		down.Contents = strings.TrimSpace(contents)
		down.Options = parseMigrateOptions(contents, downMatch[2], downMatch[3])
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
		up.Options = parseMigrateOptions(contents, upMatch[2], upMatch[3])

		down.Contents = strings.TrimSpace(contents[downStart:downEnd])
		down.Options = parseMigrateOptions(contents, downMatch[2], downMatch[3])
	}

	return up, down
}

var whitespaceRegExp = regexp.MustCompile(`\s+`)
var optionSeparatorRegExp = regexp.MustCompile(`:`)

// parseMigrationOptions parses the options portion of a migration
// block into an object that satisfies the MigrateOptions interface,
// i.e., the 'transaction:false' piece of the following:
//
//     -- migrate:up transaction:false
//     create table users (id serial, name string);
//     -- migrate:down
//     drop table users;
//
func parseMigrateOptions(contents string, begin, end int) MigrateOptions {
	mOpts := make(migrateOptions)

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
