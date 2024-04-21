// Helper package that should only be used in test files
package dbtest

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// MustParseURL parses a URL from string, and fails the test if the URL is invalid.
func MustParseURL(t *testing.T, s string) *url.URL {
	require.NotEmpty(t, s)

	u, err := url.Parse(s)
	require.NoError(t, err)

	return u
}

// GetenvOrSkip gets an environment variable, and skips the test if it is empty.
func GetenvOrSkip(t *testing.T, key string) string {
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("no %s provided", key)
	}

	return value
}

// GetenvURLOrSkip gets an environment variable, parses it as a URL,
// fails the test if the URL is invalid, and skips the test if empty.
func GetenvURLOrSkip(t *testing.T, key string) *url.URL {
	return MustParseURL(t, GetenvOrSkip(t, key))
}
