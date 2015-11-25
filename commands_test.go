package main_test

import (
	"github.com/adrianmacneil/dbmate"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestGetDatabaseUrl(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://example.org/db")

	u, err := main.GetDatabaseURL()
	require.Nil(t, err)

	require.Equal(t, "postgres", u.Scheme)
	require.Equal(t, "example.org", u.Host)
	require.Equal(t, "/db", u.Path)
}
