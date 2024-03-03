package bigquery

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

type bigQueryConfig struct {
	projectID      string
	location       string
	dataSet        string
	scopes         []string
	endpoint       string
	disableAuth    bool
	credentialFile string
}

func GetClient(ctx context.Context, uri string) (*bigquery.Client, error) {
	config, err := configFromURI(uri)
	if err != nil {
		return nil, err
	}

	opts := []option.ClientOption{option.WithScopes(config.scopes...)}
	if config.endpoint != "" {
		opts = append(opts, option.WithEndpoint(config.endpoint))
	}
	if config.disableAuth {
		opts = append(opts, option.WithoutAuthentication())
	}
	if config.credentialFile != "" {
		opts = append(opts, option.WithCredentialsFile(config.credentialFile))
	}

	client, err := bigquery.NewClient(ctx, config.projectID, opts...)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func configFromURI(uri string) (*bigQueryConfig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, invalidConnectionStringError(uri)
	}

	if u.Scheme != "bigquery" {
		return nil, fmt.Errorf("invalid prefix, expected bigquery:// got: %s", uri)
	}

	if u.Path == "" {
		return nil, invalidConnectionStringError(uri)
	}

	fields := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(fields) > 2 {
		return nil, invalidConnectionStringError(uri)
	}

	config := &bigQueryConfig{
		projectID:      u.Hostname(),
		dataSet:        fields[len(fields)-1],
		endpoint:       u.Query().Get("endpoint"),
		disableAuth:    u.Query().Get("disable_auth") == "true",
		credentialFile: u.Query().Get("credential_file"),
	}

	if len(fields) == 2 {
		config.location = fields[0]
	}

	return config, nil
}

func invalidConnectionStringError(uri string) error {
	return fmt.Errorf("invalid connection string: %s", uri)
}
