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
func (m *Migration) Parse() ([]*ParsedMigration, error) {
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
	return !strings.EqualFold(strings.TrimSpace(m["transaction"]), "false")
}

var (
	upRegExp              = regexp.MustCompile(`(?m)^--\s*(?:migrate|dbmate):up(\s*$|\s+\S+)`)
	downRegExp            = regexp.MustCompile(`(?m)^--\s*(?:migrate|dbmate):down(\s*$|\s+\S+)`)
	emptyLineRegExp       = regexp.MustCompile(`^\s*$`)
	commentLineRegExp     = regexp.MustCompile(`^\s*--`)
	directivePrefixRegExp = regexp.MustCompile(`^\s*--\s*(?:(?:migrate|dbmate):(up|down)|dbmate)(?::\s*|\s+|$)`)
	dbmateDirectiveRegExp = regexp.MustCompile(`^\s*--\s*dbmate(?::\s*|\s+|$)`)
	optionPairRegExp      = regexp.MustCompile(`([a-zA-Z0-9_-]+)\s*[:=]\s*([^\s:=]+)`)
)

// Error codes
var (
	ErrParseMissingUp      = errors.New("dbmate requires each migration to define an up block with '-- migrate:up'")
	ErrParseMissingDown    = errors.New("dbmate requires each migration to define a down block with '-- migrate:down'")
	ErrParseWrongOrder     = errors.New("dbmate requires '-- migrate:up' to appear before '-- migrate:down'")
	ErrParseUnexpectedStmt = errors.New("dbmate does not support statements preceding the '-- migrate:up' block")
	ErrParseMultipleDown   = errors.New("dbmate requires every '-- migrate:down' to be preceded by a '-- migrate:up'")
)

func parseMigrationContents(contents string) ([]*ParsedMigration, error) {
	contents = strings.TrimPrefix(contents, "\ufeff")

	allUpDirectives := upRegExp.FindAllStringIndex(contents, -1)
	if allUpDirectives == nil {
		return nil, ErrParseMissingUp
	}

	header := contents[0:allUpDirectives[0][0]]
	if statementsPrecedeMigrateBlocks(contents, allUpDirectives[0][0]) {
		return nil, ErrParseUnexpectedStmt
	}

	headerOptions := parseHeaderOptions(header)

	sectionSubstrings, err := getMigrationSectionSubstrings(contents)
	if err != nil {
		return nil, err
	}

	var migrationSections []*ParsedMigration
	for _, sectionSubstring := range sectionSubstrings {
		migrationSection, err := parseMigrationSection(sectionSubstring, headerOptions)
		if err != nil {
			return nil, err
		}
		migrationSections = append(migrationSections, migrationSection)
	}

	return migrationSections, nil
}

// parseMigrationSection parses the string contents of a migration section.
// It will return two Migration objects, the first representing the "up"
// block and the second representing the "down" block. This function
// requires that up and down blocks were defined and will otherwise
// return an error.
func parseMigrationSection(section string, defaultOptions migrationOptions) (*ParsedMigration, error) {
	upDirectiveStart, hasDefinedUpBlock := getMatchPosition(section, upRegExp)
	downDirectiveStart, hasDefinedDownBlock := getMatchPosition(section, downRegExp)

	if !hasDefinedUpBlock {
		return nil, ErrParseMissingUp
	}
	if !hasDefinedDownBlock {
		return nil, ErrParseMissingDown
	}
	if upDirectiveStart > downDirectiveStart {
		return nil, ErrParseWrongOrder
	}

	upBlock := substring(section, upDirectiveStart, downDirectiveStart)
	downBlock := substring(section, downDirectiveStart, len(section))

	parsed := ParsedMigration{
		Up:          upBlock,
		UpOptions:   parseMigrationOptions(upBlock, defaultOptions),
		Down:        downBlock,
		DownOptions: parseMigrationOptions(downBlock, defaultOptions),
	}
	return &parsed, nil
}

func stripTrailingComment(s string) string {
	if idx := strings.Index(s, "--"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "//"); idx != -1 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "/*"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func isDbmateDirective(line string) bool {
	return dbmateDirectiveRegExp.MatchString(line)
}

func parseDirectiveLineOptions(line string, options migrationOptions) {
	if !directivePrefixRegExp.MatchString(line) {
		return
	}
	remainder := directivePrefixRegExp.ReplaceAllString(line, "")
	remainder = stripTrailingComment(remainder)

	matches := optionPairRegExp.FindAllStringSubmatch(remainder, -1)
	for _, match := range matches {
		if len(match) == 3 {
			options[match[1]] = match[2]
		}
	}
}

func parseHeaderOptions(header string) migrationOptions {
	options := make(migrationOptions)
	for _, line := range strings.Split(header, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isCommentLine(trimmed) && isDbmateDirective(trimmed) {
			parseDirectiveLineOptions(trimmed, options)
		}
	}
	return options
}

// parseMigrationOptions parses the migration options out of a block
// directive into an object that implements the MigrationOptions interface.
func parseMigrationOptions(block string, defaultOptions migrationOptions) ParsedMigrationOptions {
	options := make(migrationOptions)
	for k, v := range defaultOptions {
		options[k] = v
	}

	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return options
	}

	parseDirectiveLineOptions(lines[0], options)

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isCommentLine(trimmed) {
			if isDbmateDirective(trimmed) {
				parseDirectiveLineOptions(trimmed, options)
			}
			continue
		}
		break
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

func getMigrationSectionSubstrings(contents string) ([]string, error) {
	// Regex to match blocks starting with "-- migrate:up" and ending before the next one or EOF
	allUpDirectives := upRegExp.FindAllStringIndex(contents, -1)

	if allUpDirectives == nil {
		return nil, ErrParseMissingUp
	}

	if statementsPrecedeMigrateBlocks(contents, allUpDirectives[0][0]) {
		return nil, ErrParseUnexpectedStmt
	}

	var sectionBeginEndIndices [][]int
	for i := range len(allUpDirectives) {
		start := allUpDirectives[i][0]
		var end int
		if i < len(allUpDirectives)-1 {
			end = allUpDirectives[i+1][0]
		} else {
			end = len(contents)
		}
		sectionBeginEndIndices = append(sectionBeginEndIndices, []int{start, end})
	}

	var sectionSubstrings []string
	for _, sectionBeginEnd := range sectionBeginEndIndices {
		begin, end := sectionBeginEnd[0], sectionBeginEnd[1]
		contentsSubstring := substring(contents, begin, end)
		if len(downRegExp.FindAllStringIndex(contentsSubstring, -1)) > 1 {
			return nil, ErrParseMultipleDown
		}
		sectionSubstrings = append(sectionSubstrings, contentsSubstring)
	}

	return sectionSubstrings, nil
}

func substring(s string, begin, end int) string {
	if begin == -1 || end == -1 {
		return ""
	}
	return s[begin:end]
}
