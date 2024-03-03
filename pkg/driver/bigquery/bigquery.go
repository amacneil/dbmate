package bigquery

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery"
	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	_ "gorm.io/driver/bigquery"
)

func init() {
	dbmate.RegisterDriver(NewDriver, "bigquery")
}

func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	u := config.DatabaseURL

	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		datasetID:           strings.TrimPrefix(u.Path, "/"),
		projectID:           u.Host,
		log:                 config.Log,
		databaseURL:         u,
	}
}

// Helper function that accepts an operation function and executes it with a BigQuery client.
func (drv *Driver) withBigQueryClient(operation func(context.Context, *bigquery.Client) (bool, error)) (bool, error) {
	ctx := context.Background()

	// Create a BigQuery client.
	client, err := GetClient(ctx, drv.databaseURL.String())
	if err != nil {
		return false, err
	}
	defer client.Close()

	// Execute the operation function with the client.
	result, err := operation(ctx, client)

	if err != nil {
		return result, err
	}

	return result, nil
}

// Helper function to check whether a table exists or not in a dataset
func tableExists(client *bigquery.Client, ctx context.Context, datasetID, tableName string) (bool, error) {
	table := client.Dataset(datasetID).Table(tableName)
	_, err := table.Metadata(ctx)
	if err == nil {
		return true, nil
	}
	if gError, ok := err.(*googleapi.Error); ok && gError.Code == 404 {
		return false, nil
	}
	return false, err
}

type Driver struct {
	migrationsTableName string
	datasetID           string
	projectID           string
	log                 io.Writer
	databaseURL         *url.URL
}

func (drv *Driver) CreateDatabase() error {
	exists, err := drv.DatabaseExists()
	if err != nil {
		return err
	}

	createDataset := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		err := client.Dataset(drv.datasetID).Create(ctx, &bigquery.DatasetMetadata{})
		if err != nil {
			return false, err
		}
		return true, nil
	}

	if !exists {
		_, err := drv.withBigQueryClient(createDataset)
		if err != nil {
			return err
		}
	}

	return nil
}

func (drv *Driver) CreateMigrationsTable(db *sql.DB) error {
	tableExists := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		return tableExists(client, ctx, drv.datasetID, drv.migrationsTableName)
	}

	createTable := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		table := client.Dataset(drv.datasetID).Table(drv.migrationsTableName)
		err := table.Create(ctx, &bigquery.TableMetadata{
			Schema: bigquery.Schema{
				{Name: "version", Type: bigquery.StringFieldType},
			},
		})
		if err != nil {
			return false, err
		}

		return true, nil
	}

	exists, err := drv.withBigQueryClient(tableExists)
	if err != nil {
		return err
	}

	if !exists {
		_, err := drv.withBigQueryClient(createTable)
		if err != nil {
			return err
		}
	}

	return nil
}

func (drv *Driver) DatabaseExists() (bool, error) {
	datasetExists := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		it := client.Datasets(ctx)
		for {
			dataset, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return false, err
			}
			if dataset.DatasetID == drv.datasetID {
				return true, nil
			}
		}
		return false, nil
	}

	exists, err := drv.withBigQueryClient(datasetExists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (drv *Driver) DropDatabase() error {
	datasetDrop := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		err := client.Dataset(drv.datasetID).DeleteWithContents(ctx)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	exists, err := drv.DatabaseExists()
	if err != nil {
		return err
	}

	if exists {
		_, err = drv.withBigQueryClient(datasetDrop)
		if err != nil {
			return err
		}
	}

	return nil
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

func (drv *Driver) DumpSchema(db *sql.DB) ([]byte, error) {
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
			order_type;`, drv.projectID, drv.datasetID, drv.projectID, drv.datasetID, drv.projectID, drv.datasetID)

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
		ddl, err := generateDDL(db, drv.projectID, drv.datasetID, objectName, objectType)
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
	tableExists := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		return tableExists(client, ctx, drv.datasetID, drv.migrationsTableName)
	}

	exists, err := drv.withBigQueryClient(tableExists)
	if err != nil {
		return exists, err
	}

	return exists, nil
}

func (drv *Driver) DeleteMigration(db dbutil.Transaction, version string) error {
	query := fmt.Sprintf("DELETE FROM %s.%s WHERE version = '%s';", drv.datasetID, drv.migrationsTableName, version)
	_, err := db.Exec(query)

	return err
}

func (drv *Driver) InsertMigration(db dbutil.Transaction, version string) error {
	queryTemplate := `INSERT INTO %s.%s (version) VALUES ('%s');`
	queryString := fmt.Sprintf(queryTemplate, drv.datasetID, drv.migrationsTableName, version)

	_, err := db.Exec(queryString, version)
	return err
}

func (drv *Driver) Open() (*sql.DB, error) {
	con, err := sql.Open("bigquery", drv.databaseURL.String())
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
	query := fmt.Sprintf("SELECT version FROM %s.%s ORDER BY version DESC", drv.datasetID, drv.migrationsTableName)
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
