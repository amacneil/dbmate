package bigquery

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	_ "gorm.io/driver/bigquery" // database/sql driver

	"github.com/amacneil/dbmate/v2/pkg/dbmate"
	"github.com/amacneil/dbmate/v2/pkg/dbutil"
)

type Driver struct {
	migrationsTableName string
	datasetID           string
	projectID           string
	log                 io.Writer
	databaseURL         string
	client              *bigquery.Client
	context             *context.Context
	config              *bigQueryConfig
	sqlConnectionURL    string
}

type bigQueryConfig struct {
	projectID   string
	location    string
	dataSet     string
	scopes      []string
	endpoint    string
	disableAuth bool
}

func init() {
	dbmate.RegisterDriver(NewDriver, "bigquery")
}

func NewDriver(config dbmate.DriverConfig) dbmate.Driver {
	c, _ := configFromURI(config.DatabaseURL.String())
	params := fmt.Sprintf("disable_auth=%s", strconv.FormatBool(c.disableAuth))
	if c.endpoint != "" {
		params += fmt.Sprintf("&endpoint=%s", c.endpoint)
	}
	u := fmt.Sprintf("bigquery://%s/%s?%s", c.projectID, c.dataSet, params)

	return &Driver{
		migrationsTableName: config.MigrationsTableName,
		datasetID:           c.dataSet,
		projectID:           c.projectID,
		log:                 config.Log,
		config:              c,
		databaseURL:         u,
		sqlConnectionURL:    config.DatabaseURL.String(),
	}
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
		_, err := createDataset(*drv.context, drv.client)
		if err != nil {
			return err
		}
	}

	return nil
}

func (drv *Driver) CreateMigrationsTable(*sql.DB) error {
	tableExists := func(ctx context.Context, client *bigquery.Client) (bool, error) {
		return tableExists(ctx, client, drv.datasetID, drv.migrationsTableName)
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

	exists, err := tableExists(*drv.context, drv.client)
	if err != nil {
		return err
	}

	if !exists {
		_, err := createTable(*drv.context, drv.client)
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

	exists, err := datasetExists(*drv.context, drv.client)
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
		_, err = datasetDrop(*drv.context, drv.client)
		if err != nil {
			return err
		}
	}

	return nil
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

func (drv *Driver) MigrationsTableExists(*sql.DB) (bool, error) {
	exists, err := tableExists(*drv.context, drv.client, drv.datasetID, drv.migrationsTableName)
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
	con, err := sql.Open("bigquery", drv.databaseURL)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := getClient(ctx, drv.config)
	if err != nil {
		return nil, err
	}

	drv.client = client
	drv.context = &ctx

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

func configFromURI(uri string) (*bigQueryConfig, error) {
	invalidError := func(uri string) error {
		return fmt.Errorf("invalid connection string: %s", uri)
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, invalidError(uri)
	}

	if u.Scheme != "bigquery" {
		return nil, fmt.Errorf("invalid prefix, expected bigquery:// got: %s", uri)
	}

	if u.Path == "" {
		return nil, invalidError(uri)
	}

	fields := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(fields) > 3 {
		return nil, invalidError(uri)
	}

	config := &bigQueryConfig{
		projectID:   u.Hostname(),
		dataSet:     fields[len(fields)-1],
		disableAuth: u.Query().Get("disable_auth") == "true",
	}

	if u.Port() != "" {
		config.endpoint = fmt.Sprintf("http://%s:%s", u.Hostname(), u.Port())
		config.projectID = fields[0]
	}

	if len(fields) == 2 {
		config.location = fields[0]
	}

	if len(fields) == 3 {
		config.location = fields[1]
	}

	return config, nil
}

func getClient(ctx context.Context, config *bigQueryConfig) (*bigquery.Client, error) {
	opts := []option.ClientOption{option.WithScopes(config.scopes...)}
	if config.endpoint != "" {
		opts = append(opts, option.WithEndpoint(config.endpoint))
	}
	if config.disableAuth {
		opts = append(opts, option.WithoutAuthentication())
	}

	client, err := bigquery.NewClient(ctx, config.projectID, opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
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
func tableExists(ctx context.Context, client *bigquery.Client, datasetID, tableName string) (bool, error) {
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
