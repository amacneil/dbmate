package dbmate

import (
	"database/sql"
	"fmt"
	"net/url"

	"gopkg.in/rana/ora.v4"
)

func init() {
	RegisterDriver(OracleDriver{}, "oracle")
}

// OracleDriver provides top level database functions
type OracleDriver struct {
}

func normalizeOracleURL(u *url.URL) string {
	secret, _ := u.User.Password()
	return fmt.Sprintf("%s/%s@%s%s", u.User.Username(), secret, u.Host, u.Path)
}

// Open creates a new database connection. In oracle connecting to a database means connecting to a user
// which is also a schema. Connection string format is oracle://user:password@host:port/service
func (drv OracleDriver) Open(u *url.URL) (*sql.DB, error) {
	return sql.Open(ora.Name, normalizeOracleURL(u))
}

// CreateDatabase creates a new user/schema and assigns connection, create session and granting privileges
// this requires that the creating user, as specified in URL, has `create user` privilege
func (drv OracleDriver) CreateDatabase(u *url.URL) error {
	name := u.User.Username()
	password, _ := u.User.Password()
	fmt.Printf("Creating schema: %s\n", name)

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

	_, err = db.Exec(fmt.Sprintf("create user %s identified by %s;", name, password))

	if err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("grant connect to %s;", name))

	if err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("grant create session grant any privilege to %s;", name))

	return err
}

// DropDatabase drops the specified user/schema and all objects contained in it
func (drv OracleDriver) DropDatabase(u *url.URL) error {
	name := u.User.Username()
	fmt.Printf("Dropping: %s\n", name)

	db, err := drv.Open(u)
	if err != nil {
		return err
	}
	defer mustClose(db)

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

// DatabaseExists determines whether the database exists
func (drv OracleDriver) DatabaseExists(u *url.URL) (bool, error) {
	db, err := drv.Open(u)
	if err != nil {
		return false, err
	}
	defer mustClose(db)

	var exists int8
	err = db.QueryRow("select 1 from dual").Scan(&exists)

	if err == nil {
		return true, nil
	}

	return false, err
}

// CreateMigrationsTable creates the schema_migrations table
func (drv OracleDriver) CreateMigrationsTable(db *sql.DB) error {
	var count int

	check := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	if check == nil {
		return check
	}

	_, err := db.Exec("create table schema_migrations " +
		"(version varchar2(255), primary key(version))")

	return err
}

// SelectMigrations returns a list of applied migrations
// with an optional limit (in descending order)
func (drv OracleDriver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	var query string
	baseQuery := "select version from schema_migrations"
	orderClause := "order by version desc"

	if limit >= 0 {
		query = fmt.Sprintf("%s where rownum < %d %s", baseQuery, limit+1, orderClause)
	} else {
		query = fmt.Sprintf("%s %s", baseQuery, orderClause)
	}
	rows, err := db.Query(query)
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
