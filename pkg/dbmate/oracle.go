package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/rana/ora.v4"
)

func init() {
	RegisterDriver(OracleDriver{}, "oracle")
}

// OracleDriver provides top level database functions
type OracleDriver struct {
}

func parseUserInfoFromURLQuery(u *url.URL) (string, string) {
	var targetSchema, targetPassword string

	if len(u.Query()["schema"]) > 0 {
		targetSchema = strings.ToUpper(u.Query()["schema"][0])
	}

	if len(u.Query()["passwd"]) > 0 {
		targetPassword = u.Query()["passwd"][0]
	}

	return targetSchema, targetPassword
}

func buildFromQueryParams(u *url.URL) string {
	targetSchema, targetPassword := parseUserInfoFromURLQuery(u)
	return fmt.Sprintf("%s/%s@%s%s", targetSchema, targetPassword, u.Host, u.Path)
}

func buildFromPrimaryURL(u *url.URL) string {
	secret, _ := u.User.Password()
	return fmt.Sprintf("%s/%s@%s%s", u.User.Username(), secret, u.Host, u.Path)
}

func (drv OracleDriver) openFromNormalizedURL(u *url.URL, normalize func(*url.URL) string) (*sql.DB, error) {
	return sql.Open(ora.Name, normalize(u))
}

// Open creates a new database connection. In oracle connecting to a database means connecting to a user
// which is also a schema. Connection string format is oracle://user:password@host:port/service
func (drv OracleDriver) Open(u *url.URL) (*sql.DB, error) {
	targetSchema, _ := parseUserInfoFromURLQuery(u)

	// If applicative has been specified use it
	if targetSchema != "" {
		exists, err := drv.DatabaseExists(u)
		if exists && err == nil {
			return drv.openFromNormalizedURL(u, buildFromQueryParams)
		}
	}
	return drv.openFromNormalizedURL(u, buildFromPrimaryURL)
}

// CreateDatabase creates a new user/schema and assigns connection and create session privileges along with
// the privileges specified in URL params.
// This requires that the creating user, as specified in URL, has `create user` privilege
func (drv OracleDriver) CreateDatabase(u *url.URL) error {
	name, password := parseUserInfoFromURLQuery(u)
	defaultPrivileges := []string{"connect", "create session"}
	privileges := append(defaultPrivileges, u.Query()["privileges"]...)

	fmt.Printf("Creating schema: %s\n", name)

	db, err := drv.openFromNormalizedURL(u, buildFromPrimaryURL)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(`alter session set "_oracle_script"=true`)
	if err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("create user %s identified by %s", name, password))
	if err != nil {
		return err
	}

	for _, privilege := range privileges {
		_, err = db.Exec(fmt.Sprintf("grant %s to %s", privilege, name))
		if err != nil {
			return err
		}
	}

	return nil
}

// DropDatabase drops the specified user/schema and all objects contained in it
func (drv OracleDriver) DropDatabase(u *url.URL) error {
	name, _ := parseUserInfoFromURLQuery(u)
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.openFromNormalizedURL(u, buildFromPrimaryURL)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(`alter session set "_oracle_script"=true`)
	if err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("drop user %s cascade", name))

	return err
}

// DumpSchema returns the current database schema
func (drv OracleDriver) DumpSchema(u *url.URL, db *sql.DB) ([]byte, error) {
	/* TODO reveng schema
	https://stackoverflow.com/questions/33704685/dumping-a-complete-oracle-11g-database-schema-to-a-set-of-sql-creation-statement
	http://www.orafaq.com/node/807

	refer to `dbms_metadata.get_ddl`, `dbms_metadata.get_dependent_ddl`, `all_dependencies`, `all_constraints`
	*/
	return nil, nil
}

// DatabaseExists determines whether the database exists. This requires select privilege on all_users metadata view
func (drv OracleDriver) DatabaseExists(u *url.URL) (bool, error) {
	name, _ := parseUserInfoFromURLQuery(u)

	db, err := drv.openFromNormalizedURL(u, buildFromPrimaryURL)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	var exists int8
	err = db.QueryRow("select 1 from all_users where username = :u", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}

	return exists == 1, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv OracleDriver) CreateMigrationsTable(db *sql.DB) error {
	var count int

	check := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	if check == nil {
		return check
	}

	_, err := db.Exec(`create table schema_migrations (
		version varchar2(255),
		primary key(version)
	)`)

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv OracleDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	baseQuery := "select version from schema_migrations %s order by version desc"
	limitClause := ""
	limitParam := make([]interface{}, 0)

	if limit >= 0 {
		limitClause = "where rownum < :limit"
		limitParam = append(limitParam, limit+1)
	}

	query := fmt.Sprintf(baseQuery, limitClause)

	rows, err := db.Query(query, limitParam...)
	if err != nil {
		return nil, err
	}
	defer mustClose(rows)

	migrations := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = true
	}

	return migrations, nil
}

// InsertMigration adds a new migration record
func (drv OracleDriver) InsertMigration(db Transaction, version string) error {
	_, err := db.Exec("insert into schema_migrations (version) values (:v)", version)

	return err
}

// DeleteMigration removes a migration record
func (drv OracleDriver) DeleteMigration(db Transaction, version string) error {
	_, err := db.Exec("delete from schema_migrations where version = :v", version)

	return err
}

// Ping verifies a connection to the database server. It does not verify whether the
// specified database exists.
func (drv OracleDriver) Ping(u *url.URL) error {
	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	err = db.Ping()
	if err == nil {
		return nil
	}

	fmt.Println(err)
	return err
}
