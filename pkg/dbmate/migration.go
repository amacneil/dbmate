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

var upRegExp = regexp.MustCompile(`(?m)^--\s*migrate:up(\s*$|\s+\S+)`)
var downRegExp = regexp.MustCompile(`(?m)^--\s*migrate:down(\s*$|\s+\S+)$`)
var emptyLineRegExp = regexp.MustCompile(`^\s*$`)
var commentLineRegExp = regexp.MustCompile(`^\s*--`)
var whitespaceRegExp = regexp.MustCompile(`\s+`)
var optionSeparatorRegExp = regexp.MustCompile(`:`)
var blockDirectiveRegExp = regexp.MustCompile(`^--\s*migrate:[up|down]]`)

// parseMigrationContents parses the string contents of a migration.
// It will return two Migration objects, the first representing the "up"
// block and the second representing the "down" block. This function
// requires that at least an up block was defined and will otherwise
// return an error.
func parseMigrationContents(contents string) (Migration, Migration, error) {
	up := NewMigration()
	down := NewMigration()

	upDirectiveStart, upDirectiveEnd, hasDefinedUpBlock := getMatchPositions(contents, upRegExp)
	downDirectiveStart, downDirectiveEnd, hasDefinedDownBlock := getMatchPositions(contents, downRegExp)

	if !hasDefinedUpBlock {
		return up, down, fmt.Errorf("dbmate requires each migration to define an up bock with '-- migrate:up'")
	} else if statementsPrecedeMigrateBlocks(contents, upDirectiveStart, downDirectiveStart) {
		return up, down, fmt.Errorf("dbmate does not support statements defined outside of the '-- migrate:up' or '-- migrate:down' blocks")
	}

	upEnd := len(contents)
	downEnd := len(contents)

	if hasDefinedDownBlock && upDirectiveStart < downDirectiveStart {
		upEnd = downDirectiveStart
	} else if hasDefinedDownBlock && upDirectiveStart > downDirectiveStart {
		downEnd = upDirectiveStart
	} else {
		downEnd = -1
	}

	upDirective := substring(contents, upDirectiveStart, upDirectiveEnd)
	downDirective := substring(contents, downDirectiveStart, downDirectiveEnd)

	up.Options = parseMigrationOptions(upDirective)
	up.Contents = substring(contents, upDirectiveStart, upEnd)

	down.Options = parseMigrationOptions(downDirective)
	down.Contents = substring(contents, downDirectiveStart, downEnd)

	return up, down, nil
}

// parseMigrationOptions parses the migration options out of a block
// directive into an object that implements the MigrationOptions interface.
//
// For example:
//
//     fmt.Printf("%#v", parseMigrationOptions("-- migrate:up transaction:false"))
//     // migrationOptions{"transaction": "false"}
//
func parseMigrationOptions(contents string) MigrationOptions {
	options := make(migrationOptions)

	// strip away the -- migrate:[up|down] part
	contents = blockDirectiveRegExp.ReplaceAllString(contents, "")

	// remove leading and trailing whitespace
	contents = strings.TrimSpace(contents)

	// return empty options if nothing is left to parse
	if contents == "" {
		return options
	}

	// split the options string into pairs, e.g. "transaction:false foo:bar" -> []string{"transaction:false", "foo:bar"}
	stringPairs := whitespaceRegExp.Split(contents, -1)

	for _, stringPair := range stringPairs {
		// split stringified pair into key and value pairs, e.g. "transaction:false" -> []string{"transaction", "false"}
		pair := optionSeparatorRegExp.Split(stringPair, -1)

		// if the syntax is well-formed, then store the key and value pair in options
		if len(pair) == 2 {
			options[pair[0]] = pair[1]
		}
	}

	return options
}

// statementsPrecedeMigrateBlocks inspects the contents between the first character
// of a string and the index of the first block directive to see if there are any statements
// defined outside of the block directive. It'll return true if it finds any such statements.
//
// For example:
//
// This will return false:
//
// statementsPrecedeMigrateBlocks(`-- migrate:up
// create table users (id serial);
// `, 0, -1)
//
// This will return true:
//
// statementsPrecedeMigrateBlocks(`create type status_type as enum('active', 'inactive');
// -- migrate:up
// create table users (id serial, status status_type);
// `, 54, -1)
//
func statementsPrecedeMigrateBlocks(contents string, upDirectiveStart, downDirectiveStart int) bool {
	until := upDirectiveStart

	if downDirectiveStart > -1 {
		until = min(upDirectiveStart, downDirectiveStart)
	}

	lines := strings.Split(contents[0:until], "\n")

	for _, line := range lines {
		if isEmptyLine(line) || isCommentLine(line) {
			continue
		}
		return true
	}

	return false
}

// isEmptyLine will return true if the line has no
// characters or if all the characters are whitespace characters
func isEmptyLine(s string) bool {
	return emptyLineRegExp.MatchString(s)
}

// isCommentLine will return true if the line is a SQL comment
func isCommentLine(s string) bool {
	return commentLineRegExp.MatchString(s)
}

func getMatchPositions(s string, re *regexp.Regexp) (int, int, bool) {
	match := re.FindStringIndex(s)
	if match == nil {
		return -1, -1, false
	}
	return match[0], match[1], true
}

func substring(s string, begin, end int) string {
	if begin == -1 || end == -1 {
		return ""
	}
	return s[begin:end]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
