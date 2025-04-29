package bigquery

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"unsafe"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	_ "gorm.io/driver/bigquery" // database/sql driver

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

func init() {
	dbmate.RegisterDriver(NewDriver, "bigquery")
}

type Driver struct {
	migrationsTableName string
	databaseURL         *url.URL
	log                 io.Writer
}

func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		databaseURL:         config.DatabaseURL,
		log:                 config.Log,
	}
}

func (drv *Driver) CreateDatabase() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	exists, err := drv.DatabaseExists()
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		config := getConfig(driverConn)

		return client.Dataset(config.dataSet).Create(ctx, &bigquery.DatasetMetadata{
			Location: config.location,
		})
	})
}

func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		config := getConfig(driverConn)

		exists, err := tableExists(client, config.dataSet, drv.migrationsTableName)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		return client.Dataset(config.dataSet).Table(drv.migrationsTableName).Create(ctx, &bigquery.TableMetadata{
			Schema: bigquery.Schema{
				&bigquery.FieldSchema{
					Name: "version",
					Type: bigquery.StringFieldType,
				},
			},
		})
	})
}

func (drv *Driver) DatabaseExists() (bool, error) {
	db, err := drv.Open()
	if err != nil {
		return false, err
	}
	defer dbutil.MustClose(db)

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	var exists bool
	err = conn.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		config := getConfig(driverConn)

		it := client.Datasets(ctx)
		for {
			dataset, err := it.Next()
			if err == iterator.Done {
				exists = false
				return nil
			}
			if err != nil {
				return err
			}
			if dataset.DatasetID == config.dataSet {
				exists = true
				return nil
			}
		}
	})

	return exists, err
}

func (drv *Driver) DropDatabase() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	exists, err := drv.DatabaseExists()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		config := getConfig(driverConn)

		return client.Dataset(config.dataSet).DeleteWithContents(ctx)
	})
}

func (drv *Driver) schemaDump(db *sql.DB) ([]byte, error) {
	// build schema migrations table data
	var buf bytes.Buffer
	buf.WriteString("\n--\n-- Database schema\n--\n\n")

	config, err := drv.getConfig(db)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT table_name AS object_name, 'TABLE' AS object_type, ddl
		FROM `+"`%s.%s.INFORMATION_SCHEMA.TABLES`"+`
		UNION ALL
		SELECT routine_name AS object_name, 'FUNCTION' AS object_type, ddl
		FROM `+"`%s.%s.INFORMATION_SCHEMA.ROUTINES`"+`
		ORDER BY CASE object_type
			WHEN 'TABLE' THEN 1
			WHEN 'FUNCTION' THEN 2
			ELSE 3
		END;`,
		config.projectID, config.dataSet,
		config.projectID, config.dataSet,
	)

	// Execute the query
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying objects: %v", err)
	}
	defer dbutil.MustClose(rows)

	// Iterate over the results and generate DDL for each object
	for rows.Next() {
		var objectName, objectType, ddl string
		if err := rows.Scan(&objectName, &objectType, &ddl); err != nil {
			return nil, fmt.Errorf("error scanning object: %v", err)
		}

		buf.WriteString(ddl + "\n")
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating objects: %v", err)
	}

	return buf.Bytes(), nil
}

func (drv *Driver) schemaMigrationsDump(db *sql.DB) ([]byte, error) {
	migrationsTable := drv.migrationsTableName

	// load applied migrations
	migrations, err := dbutil.QueryColumn(db,
		fmt.Sprintf("select version from %s order by version asc", migrationsTable))
	if err != nil {
		return nil, err
	}

	// build schema migrations table data
	var buf bytes.Buffer
	buf.WriteString("\n--\n-- Dbmate schema migrations\n--\n\n")

	if len(migrations) > 0 {
		buf.WriteString(
			fmt.Sprintf("INSERT INTO %s (version) VALUES\n    ('", migrationsTable) +
				strings.Join(migrations, "'),\n    ('") +
				"');\n")
	}

	return buf.Bytes(), nil
}

func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
	schema, err := drv.schemaDump(db)
	if err != nil {
		return nil, err
	}

	migrations, err := drv.schemaMigrationsDump(db)
	if err != nil {
		return nil, err
	}

	return append(schema, migrations...), nil
}

func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	var exists bool
	err = conn.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		config := getConfig(driverConn)
		exists, err = tableExists(client, config.dataSet, drv.migrationsTableName)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return exists, err
	}

	return exists, nil
}

func (drv *Driver) DeleteMigration(util dbutil.Transaction, version string) error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	config, err := drv.getConfig(db)
	if err != nil {
		return err
	}

	query := fmt.Sprintf("DELETE FROM %s.%s WHERE version = '%s';", config.dataSet, drv.migrationsTableName, version)
	_, err = util.Exec(query)
	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) InsertMigration(_ dbutil.Transaction, version string) error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	config, err := drv.getConfig(db)
	if err != nil {
		return err
	}

	queryTemplate := `INSERT INTO %s.%s (version) VALUES ('%s');`
	queryString := fmt.Sprintf(queryTemplate, config.dataSet, drv.migrationsTableName, version)
	_, err = db.Exec(queryString, version)
	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) Open() (*sql.DB, error) {
	return sql.Open("bigquery", connectionString(drv.databaseURL))
}

func (drv *Driver) Ping() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	return db.Ping()
}

func (*Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}

func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	config, err := drv.getConfig(db)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT version FROM %s.%s ORDER BY version DESC", config.dataSet, drv.migrationsTableName)
	if limit >= 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer dbutil.MustClose(rows)

	migrations := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}

		migrations[version] = true
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return migrations, nil
}

// Helper function to check whether a table exists or not in a dataset
func tableExists(client *bigquery.Client, datasetID, tableName string) (bool, error) {
	table := client.Dataset(datasetID).Table(tableName)
	_, err := table.Metadata(context.Background())
	if err == nil {
		return true, nil
	}
	if gError, ok := err.(*googleapi.Error); ok && gError.Code == 404 {
		return false, nil
	}
	return false, err
}

func connectionString(u *url.URL) string {
	return u.String()
}

// nolint:unused
func connectionStringOld(u *url.URL) string {
	//if connecting to emulator with host:port format
	if u.Port() != "" {
		paths := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")

		newURL := &url.URL{
			Scheme: u.Scheme,
			Host:   paths[0],
		}

		params := url.Values{}
		if u.Query().Get("disable_auth") == "true" {
			params.Set("disable_auth", "true")
		}
		params.Set("endpoint", fmt.Sprintf("http://%s:%s", u.Hostname(), u.Port()))

		if len(paths) == 3 {
			// bigquery://host:port/project/location/dataset
			newURL.Path += "/" + paths[1]
			newURL.Path += "/" + paths[2]
		} else {
			// bigquery://host:port/project/dataset
			newURL.Path += "/" + paths[1]
		}

		newURL.RawQuery = params.Encode()

		return newURL.String()
	}

	//connecting to GCP BigQuery, drop all query strings
	newURL := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}

	return newURL.String()
}

func (drv *Driver) getClient(db *sql.DB) (*bigquery.Client, error) {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var client *bigquery.Client

	err = conn.Raw(func(driverConn any) error {
		client = getClient(driverConn)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func getClient(driverConn any) *bigquery.Client {
	value := reflect.ValueOf(driverConn).Elem().FieldByName("client")
	value = reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr()))
	return value.Elem().Interface().(*bigquery.Client)
}

// As the `bigQueryConfig` struct is unexported from `go-gorm/bigquery`,
// we need to maintain a copy here and access it through reflection.
//
// See: https://github.com/go-gorm/bigquery/blob/74582cba0726b82b8a59990fee4064e059e88c9b/driver/driver.go#L18-L27
//
// nolint:unused
type bigQueryConfig struct {
	projectID      string
	location       string
	dataSet        string
	scopes         []string
	endpoint       string
	disableAuth    bool
	credentialFile string
	credentialJSON []byte
}

func (drv *Driver) getConfig(db *sql.DB) (*bigQueryConfig, error) {
	conn, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var config *bigQueryConfig

	err = conn.Raw(func(driverConn any) error {
		config = getConfig(driverConn)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return config, nil
}

func getConfig(driverConn any) *bigQueryConfig {
	value := reflect.ValueOf(driverConn).Elem().FieldByName("config")
	value = reflect.NewAt(reflect.TypeOf(bigQueryConfig{}), unsafe.Pointer(value.UnsafeAddr()))
	return value.Interface().(*bigQueryConfig)
}
