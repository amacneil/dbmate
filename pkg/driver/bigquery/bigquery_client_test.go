package bigquery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func getCredFile(t *testing.T) string {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	// Construct the absolute path to the credential file
	return filepath.Join(cwd, "test_cred.json")
}

func assert(cases []string, t *testing.T, isValid bool) {
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			ctx := context.Background()
			_, err := GetClient(ctx, c)
			if isValid {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidURIsWithoutCred(t *testing.T) {
	cases := [4]string{
		"bigquery://projectid/dataset?disable_auth=true",
		"bigquery://projectid/location/dataset?disable_auth=true",
		"bigquery://projectid/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&disable_auth=true",
		"bigquery://projectid/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&disable_auth=true",
	}

	assert(cases[:], t, true)
}

func TestValidURIsWithCred(t *testing.T) {
	credentialFile := getCredFile(t)

	cases := [4]string{
		"bigquery://projectid/dataset?credential_file=" + credentialFile,
		"bigquery://projectid/location/dataset?credential_file=" + credentialFile,
		"bigquery://projectid/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&credential_file=" + credentialFile,
		"bigquery://projectid/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050&credential_file=" + credentialFile,
	}

	assert(cases[:], t, true)
}

func TestValidURIsWithEnvCred(t *testing.T) {
	cases := [4]string{
		"bigquery://projectid/dataset",
		"bigquery://projectid/location/dataset",
		"bigquery://projectid/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050",
		"bigquery://projectid/location/dataset?endpoint=http%3A%2F%2F0.0.0.0%3A9050",
	}

	env := "GOOGLE_APPLICATION_CREDENTIALS"
	os.Setenv(env, getCredFile(t))
	defer os.Unsetenv(env)

	assert(cases[:], t, true)
}

func TestInvalidURIs(t *testing.T) {
	cases := [5]string{
		"bigquery://projectiddataset",
		"bigquery://projectid/location/dataset/unknown",
		"bigquery://projectid/dataset?endpoint=https://localhost:3000",
		"bigquery://projectid/location/dataset?",
		"bigquery://projectid/dataset?credential_file=does/not/exist",
	}

	assert(cases[:], t, false)
}
