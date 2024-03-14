package bigquery

import (
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
	con, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer con.Close()

	err = con.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		dataset := getDataset(driverConn)
		err := client.Dataset(dataset).Create(ctx, &bigquery.DatasetMetadata{})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	ctx := context.Background()

	con, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer con.Close()

	//check if the table exists
	var exists bool

	err = con.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		dataset := getDataset(driverConn)
		exists, err = tableExists(client, dataset, drv.migrationsTableName)

		if err != nil {
			return err
		}

		if !exists {
			table := client.Dataset(dataset).Table(drv.migrationsTableName)
			err := table.Create(ctx, &bigquery.TableMetadata{
				Schema: bigquery.Schema{
					{
						Name: "version",
						Type: bigquery.StringFieldType,
					},
				},
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) DatabaseExists() (bool, error) {
	db, err := drv.Open()
	if err != nil {
		return false, err
	}
	defer dbutil.MustClose(db)

	ctx := context.Background()

	con, err := db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer con.Close()

	var exists bool

	err = con.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		datasetID := getDataset(driverConn)
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
			if dataset.DatasetID == datasetID {
				exists = true
				return nil
			}
		}
	})

	if err != nil {
		return exists, err
	}

	return exists, nil
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

	con, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer con.Close()

	err = con.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		dataset := getDataset(driverConn)
		err := client.Dataset(dataset).DeleteWithContents(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
	ctx := context.Background()
	con, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer con.Close()

	var projectID, datasetID string

	err = con.Raw(func(driverConn any) error {
		projectID = getProjectID(driverConn)
		datasetID = getDataset(driverConn)
		return nil
	})

	if err != nil {
		return nil, err
	}

	var script []byte

	query := fmt.Sprintf(`
		SELECT
			table_name,
			table_type,
			1 AS order_type
		FROM
			`+"`%s.%s.INFORMATION_SCHEMA.TABLES`"+`
		WHERE
			table_type = 'BASE TABLE'
		UNION ALL
		SELECT
			table_name,
			table_type,
			2 AS order_type
		FROM
			`+"`%s.%s.INFORMATION_SCHEMA.TABLES`"+`
		WHERE
			table_type = 'VIEW'
		UNION ALL
		SELECT
			routine_name AS table_name,
			'FUNCTION' AS table_type,
			3 AS order_type
		FROM
			`+"`%s.%s.INFORMATION_SCHEMA.ROUTINES`"+`
		ORDER BY
			order_type;`, projectID, datasetID, projectID, datasetID, projectID, datasetID)

	// Execute the query
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying objects: %v", err)
	}
	defer dbutil.MustClose(rows)

	// Iterate over the results and generate DDL for each object
	for rows.Next() {
		var objectName, objectType string
		var orderType int
		if err := rows.Scan(&objectName, &objectType, &orderType); err != nil {
			return nil, fmt.Errorf("error scanning object: %v", err)
		}

		// Generate DDL for the object
		ddl, err := generateDDL(db, projectID, datasetID, objectName, objectType)
		if err != nil {
			return nil, fmt.Errorf("error generating DDL for %s %s: %v", objectName, objectType, err)
		}

		// Append the DDL to the script
		script = append(script, []byte(ddl)...)
		script = append(script, []byte("\n\n")...)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating objects: %v", err)
	}

	return script, nil
}

func (drv *Driver) MigrationsTableExists(db *sql.DB) (bool, error) {
	var exists bool

	ctx := context.Background()

	con, err := db.Conn(ctx)
	if err != nil {
		return exists, err
	}
	defer con.Close()

	err = con.Raw(func(driverConn any) error {
		client := getClient(driverConn)
		dataset := getDataset(driverConn)
		exists, err = tableExists(client, dataset, drv.migrationsTableName)
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

	ctx := context.Background()

	con, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer con.Close()

	var dataset string

	err = con.Raw(func(driverConn any) error {
		dataset = getDataset(driverConn)
		return nil
	})

	if err != nil {
		return err
	}

	query := fmt.Sprintf("DELETE FROM %s.%s WHERE version = '%s';", dataset, drv.migrationsTableName, version)
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

	ctx := context.Background()
	con, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer con.Close()

	var dataset string

	err = con.Raw(func(driverConn any) error {
		dataset = getDataset(driverConn)
		return nil
	})

	if err != nil {
		return err
	}

	queryTemplate := `INSERT INTO %s.%s (version) VALUES ('%s');`
	queryString := fmt.Sprintf(queryTemplate, dataset, drv.migrationsTableName, version)

	_, err = db.Exec(queryString, version)

	if err != nil {
		return err
	}

	return nil
}

func (drv *Driver) Open() (*sql.DB, error) {
	connString := connectionString(drv.databaseURL)
	con, err := sql.Open("bigquery", connString)
	if err != nil {
		return nil, err
	}

	return con, err
}

func (drv *Driver) Ping() error {
	db, err := drv.Open()
	if err != nil {
		return err
	}
	defer dbutil.MustClose(db)

	err = db.Ping()

	return err
}

func (*Driver) QueryError(query string, err error) error {
	return &dbmate.QueryError{Err: err, Query: query}
}

func (drv *Driver) SelectMigrations(db *sql.DB, limit int) (map[string]bool, error) {
	ctx := context.Background()
	con, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer con.Close()

	var dataset string

	err = con.Raw(func(driverConn any) error {
		dataset = getDataset(driverConn)
		return nil
	})

	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT version FROM %s.%s ORDER BY version DESC", dataset, drv.migrationsTableName)
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

func generateDDL(db *sql.DB, projectID, datasetID, objectName, objectType string) (string, error) {
	var ddl string

	// Query to retrieve object information
	switch objectType {
	case "BASE TABLE":
		// Query to retrieve column information for tables
		query := fmt.Sprintf(`
			SELECT
				column_name,
				data_type,
				is_nullable
			FROM
				`+"`%s.%s.INFORMATION_SCHEMA.COLUMNS`"+`
			WHERE
				table_name = '%s'`, projectID, datasetID, objectName)

		// Execute the query
		rows, err := db.Query(query)
		if err != nil {
			return "", err
		}
		defer dbutil.MustClose(rows)

		// Check for any error that occurred during the query execution
		if err := rows.Err(); err != nil {
			return "", err
		}

		// Generate DDL for tables
		for rows.Next() {
			var columnName, dataType, isNullable string
			if err := rows.Scan(&columnName, &dataType, &isNullable); err != nil {
				return "", err
			}
			if isNullable == "YES" {
				ddl += fmt.Sprintf("\t%s %s,\n", columnName, dataType)
			} else {
				ddl += fmt.Sprintf("\t%s %s %s,\n", columnName, dataType, "NOT NULL")
			}
		}
		if ddl != "" {
			ddl = fmt.Sprintf("CREATE TABLE `%s.%s.%s` (\n%s);", projectID, datasetID, objectName, ddl)
		}
	case "VIEW":
		// Query to retrieve view definition
		query := fmt.Sprintf(`
			SELECT
				view_definition
			FROM
				`+"`%s.%s.INFORMATION_SCHEMA.VIEWS`"+`
			WHERE
				table_name = '%s'`, projectID, datasetID, objectName)

		// Execute the query
		row := db.QueryRow(query)
		if err := row.Scan(&ddl); err != nil {
			return "", err
		}
		ddl = fmt.Sprintf("CREATE VIEW `%s.%s.%s` AS\n%s;", projectID, datasetID, objectName, ddl)
		ddl = strings.ReplaceAll(ddl, "\n", "\n\t")
	case "FUNCTION":
		// Query to retrieve function definition
		definitionQuery := fmt.Sprintf(`
			SELECT
				routine_definition
			FROM
				`+"`%s.%s.INFORMATION_SCHEMA.ROUTINES`"+`
			WHERE
				routine_name = '%s'`, projectID, datasetID, objectName)

		// Execute the query to fetch function definition
		definitionRow := db.QueryRow(definitionQuery)
		if err := definitionRow.Scan(&ddl); err != nil {
			return "", err
		}

		// Query to retrieve function parameters
		paramQuery := fmt.Sprintf(`
			SELECT
				parameter_name,
				data_type,
				ordinal_position
			FROM
				`+"`%s.%s.INFORMATION_SCHEMA.PARAMETERS`"+`
			WHERE
				specific_name = '%s'`, projectID, datasetID, objectName)

		// Execute the query to fetch function parameters
		paramRows, err := db.Query(paramQuery)
		if err != nil {
			return "", err
		}
		defer dbutil.MustClose(paramRows)

		// Check for any error that occurred during the query execution
		if err := paramRows.Err(); err != nil {
			return "", err
		}

		// Construct function parameters list
		var paramList []string
		var returnType string
		for paramRows.Next() {
			var paramName sql.NullString
			var dataType string
			var ordinalPosition int
			if err := paramRows.Scan(&paramName, &dataType, &ordinalPosition); err != nil {
				return "", err
			}
			if ordinalPosition == 0 {
				returnType = dataType
			} else {
				paramList = append(paramList, fmt.Sprintf("%s %s", paramName.String, dataType))
			}
		}
		params := strings.Join(paramList, ", ")

		// Construct the function DDL with parameters
		ddl = fmt.Sprintf("CREATE FUNCTION `%s.%s.%s` (%s) RETURNS %s AS (\n\t%s\n);", projectID, datasetID, objectName, params, returnType, ddl)
	default:
		return "", fmt.Errorf("unsupported object type: %s", objectType)
	}

	return ddl, nil
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

func getClient(con any) *bigquery.Client {
	value := reflect.ValueOf(con).Elem().FieldByName("client")
	value = reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
	client := value.Interface().(*bigquery.Client)
	return client
}

func getConfigValue(con any, field string) reflect.Value {
	connValue := reflect.ValueOf(con).Elem()
	configField := connValue.FieldByName("config")
	return configField.FieldByName(field)
}

func getProjectID(con any) string {
	return getConfigValue(con, "projectID").String()
}

func getDataset(con any) string {
	return getConfigValue(con, "dataSet").String()
}
