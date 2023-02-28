package dbmate

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

// Migration represents an available migration and status
type Migration struct {
	Applied  bool
	FileName string
	FilePath string
	FS       fs.FS
	Version  string
}

func (m *Migration) readFile() (string, error) {
	if m.FS == nil {
		bytes, err := os.ReadFile(m.FilePath)
		return string(bytes), err
	}

	bytes, err := fs.ReadFile(m.FS, m.FilePath)
	return string(bytes), err
}

// Parse a migration
func (m *Migration) Parse() (*ParsedMigration, error) {
	contents, err := m.readFile()
	if err != nil {
		return nil, err
	}

	return parseMigrationContents(contents)
}

// ParsedMigration contains the migration contents and options
type ParsedMigration struct {
	Up          string
	UpOptions   ParsedMigrationOptions
	Down        string
	DownOptions ParsedMigrationOptions
}

// ParsedMigrationOptions is an interface for accessing migration options
type ParsedMigrationOptions interface {
	Transaction() bool
}

type migrationOptions map[string]string

// Transaction returns whether or not this migration should run in a transaction
// Defaults to true.
func (m migrationOptions) Transaction() bool {
	return m["transaction"] != "false"
}

var (
	upRegExp              = regexp.MustCompile(`(?m)^--\s*migrate:up(\s*$|\s+\S+)`)
	downRegExp            = regexp.MustCompile(`(?m)^--\s*migrate:down(\s*$|\s+\S+)$`)
	emptyLineRegExp       = regexp.MustCompile(`^\s*$`)
	commentLineRegExp     = regexp.MustCompile(`^\s*--`)
	whitespaceRegExp      = regexp.MustCompile(`\s+`)
	optionSeparatorRegExp = regexp.MustCompile(`:`)
	blockDirectiveRegExp  = regexp.MustCompile(`^--\s*migrate:[up|down]]`)
)

// Error codes
var (
	ErrParseMissingUp      = errors.New("dbmate requires each migration to define an up block with '-- migrate:up'")
	ErrParseMissingDown    = errors.New("dbmate requires each migration to define a down block with '-- migrate:down'")
	ErrParseWrongOrder     = errors.New("dbmate requires '-- migrate:up' to appear before '-- migrate:down'")
	ErrParseUnexpectedStmt = errors.New("dbmate does not support statements preceding the '-- migrate:up' block")
)

// parseMigrationContents parses the string contents of a migration.
// It will return two Migration objects, the first representing the "up"
// block and the second representing the "down" block. This function
// requires that at least an up block was defined and will otherwise
// return an error.
func parseMigrationContents(contents string) (*ParsedMigration, error) {
	upDirectiveStart, hasDefinedUpBlock := getMatchPosition(contents, upRegExp)
	downDirectiveStart, hasDefinedDownBlock := getMatchPosition(contents, downRegExp)

	if !hasDefinedUpBlock {
		return nil, ErrParseMissingUp
	}
	if !hasDefinedDownBlock {
		return nil, ErrParseMissingDown
	}
	if upDirectiveStart > downDirectiveStart {
		return nil, ErrParseWrongOrder
	}
	if statementsPrecedeMigrateBlocks(contents, upDirectiveStart) {
		return nil, ErrParseUnexpectedStmt
	}

	upBlock := substring(contents, upDirectiveStart, downDirectiveStart)
	downBlock := substring(contents, downDirectiveStart, len(contents))

	parsed := ParsedMigration{
		Up:          upBlock,
		UpOptions:   parseMigrationOptions(upBlock),
		Down:        downBlock,
		DownOptions: parseMigrationOptions(downBlock),
	}
	return &parsed, nil
}

// parseMigrationOptions parses the migration options out of a block
// directive into an object that implements the MigrationOptions interface.
//
// For example:
//
//	fmt.Printf("%#v", parseMigrationOptions("-- migrate:up transaction:false"))
//	// migrationOptions{"transaction": "false"}
func parseMigrationOptions(contents string) ParsedMigrationOptions {
	options := make(migrationOptions)

	// remove everything after first newline
	contents = strings.SplitN(contents, "\n", 2)[0]

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
func statementsPrecedeMigrateBlocks(contents string, upDirectiveStart int) bool {
	lines := strings.Split(contents[0:upDirectiveStart], "\n")

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

func getMatchPosition(s string, re *regexp.Regexp) (int, bool) {
	match := re.FindStringIndex(s)
	if match == nil {
		return -1, false
	}
	return match[0], true
}

func substring(s string, begin, end int) string {
	if begin == -1 || end == -1 {
		return ""
	}
	return s[begin:end]
}
