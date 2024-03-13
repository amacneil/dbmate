package dbmate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
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
	downRegExp            = regexp.MustCompile(`(?m)^--\s*migrate:down(\s*$|\s+\S+)`)
	emptyLineRegExp       = regexp.MustCompile(`^\s*$`)
	commentLineRegExp     = regexp.MustCompile(`^\s*--`)
	whitespaceRegExp      = regexp.MustCompile(`\s+`)
	optionSeparatorRegExp = regexp.MustCompile(`:`)
	blockDirectiveRegExp  = regexp.MustCompile(`^--\s*migrate:(up|down)`)
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

type migrationRange struct {
	start *string
	end   *string
}

type migrationTracking struct {
	lengthOfMigrations          int
	currentMigration            int
	singleMigrationsLookup      map[string]migrationRange
	activeMigrationsLookup      map[string]migrationRange
	inactiveMigrationsLookup    map[string]migrationRange
	inactiveEndMigrationsLookup map[string]migrationRange
}

func filterMigrations(givenMigrations []string, migrations []Migration) ([]Migration, error) {
	tracking := newMigrationTracking(givenMigrations, len(migrations))
	if tracking == nil {
		return migrations, nil
	}
	filteredMigrations := make([]Migration, 0, len(migrations))
	for _, migration := range migrations {
		if !tracking.addNext(migration) {
			continue
		}
		filteredMigrations = append(filteredMigrations, migration)
	}
	notFound, err := tracking.givenMigrationsNotFound()
	if err != nil {
		return nil, err
	}
	if len(notFound) > 0 {
		return nil, fmt.Errorf("%w `%v` %d", ErrMigrationNotFound, notFound, len(notFound))
	}
	return filteredMigrations, nil
}

// given migrations is a list of migrations
// a migration can be either the migration version or the migration file name
// Ranges are supported as well.
//
// dbmate -m ...version # everything before and including version
// dbmate -m version... # everything starting at version and after
// dbmate -m version...version2 # everything starting at version and ending at version2
func newMigrationTracking(givenMigrations []string, lengthOfMigrations int) *migrationTracking {
	if len(givenMigrations) == 0 {
		return nil
	}

	singleMigrationsLookup := make(map[string]migrationRange)
	inactiveMigrationsLookup := make(map[string]migrationRange)
	inactiveEndMigrationsLookup := make(map[string]migrationRange)
	activeMigrationsLookup := make(map[string]migrationRange)

	for _, given := range givenMigrations {
		if !strings.Contains(given, "...") {
			singleMigrationsLookup[given] = migrationRange{}
			continue
		}

		mr := migrationRange{}
		split := strings.Split(given, "...")
		start := split[0]
		if start == "" {
			mr.start = nil
		} else {
			mr.start = &start
		}
		if len(split) > 1 {
			mr.end = &split[1]
		}
		// Empty range means all migrations
		if mr.start == nil && mr.end == nil {
			return nil
		}
		if mr.start == nil {
			activeMigrationsLookup[*mr.end] = mr
		} else {
			inactiveMigrationsLookup[*mr.start] = mr
			if mr.end != nil {
				inactiveEndMigrationsLookup[*mr.end] = mr
			}
		}
	}

	return &migrationTracking{
		currentMigration:            0,
		lengthOfMigrations:          lengthOfMigrations,
		singleMigrationsLookup:      singleMigrationsLookup,
		activeMigrationsLookup:      activeMigrationsLookup,
		inactiveMigrationsLookup:    inactiveMigrationsLookup,
		inactiveEndMigrationsLookup: inactiveEndMigrationsLookup,
	}
}

func (ar *migrationTracking) givenMigrationsNotFound() ([]string, error) {
	notFound := make([]string, 0)
	for m := range ar.singleMigrationsLookup {
		notFound = append(notFound, m)
	}
	for _, mr := range ar.inactiveMigrationsLookup {
		notFound = append(notFound, *mr.start)
	}
	for key, mr := range ar.activeMigrationsLookup {
		if key != "" && mr.end != nil {
			if _, ok := ar.inactiveEndMigrationsLookup[*mr.end]; !ok {
				return notFound, fmt.Errorf("%w %v...%v because end comes before start- their order should be reversed", ErrMigrationNotFound, *mr.start, *mr.end)
			}
			notFound = append(notFound, *mr.end)
		}
	}
	return notFound, nil
}

func (ar *migrationTracking) retrieveMigration(mapping map[string]migrationRange, migration Migration) (migrationRange, bool) {
	if m, ok := mapping[migration.Version]; ok {
		delete(mapping, migration.Version)
		return m, true
	}
	if m, ok := mapping[migration.FileName]; ok {
		delete(mapping, migration.FileName)
		return m, true
	}

	// support using a numeric index with a leading plus
	positiveStr := "+" + strconv.FormatInt(int64(ar.currentMigration), 10)
	if m, ok := mapping[positiveStr]; ok {
		delete(mapping, positiveStr)
		return m, true
	}
	// the numeric index can be negative (this is more useful than positive)
	negative := ar.currentMigration - ar.lengthOfMigrations
	negativeStr := strconv.FormatInt(int64(negative), 10)
	if m, ok := mapping[negativeStr]; ok {
		delete(mapping, negativeStr)
		return m, true
	}

	return migrationRange{}, false
}

// addNext returns true if the migration is selected
func (ar *migrationTracking) addNext(migration Migration) bool {
	ar.currentMigration++
	selected := false
	if _, ok := ar.retrieveMigration(ar.singleMigrationsLookup, migration); ok {
		selected = true
	}
	if _, ok := ar.retrieveMigration(ar.activeMigrationsLookup, migration); ok {
		selected = true
	}
	// Tracking just for error handling purposes
	_, _ = ar.retrieveMigration(ar.inactiveEndMigrationsLookup, migration)

	// transfer from inactive to active
	if mr, ok := ar.retrieveMigration(ar.inactiveMigrationsLookup, migration); ok {
		if mr.end == nil {
			// active for all the rest of the migrations
			ar.activeMigrationsLookup[""] = mr
		} else {
			// check to see if start and end are the same
			if *mr.end != migration.Version && *mr.end != migration.FileName {
				ar.activeMigrationsLookup[*mr.end] = mr
			}
		}
	}

	if len(ar.activeMigrationsLookup) > 0 {
		return true
	}

	return selected
}
