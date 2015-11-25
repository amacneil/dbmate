package shared_test

import (
	"github.com/adrianmacneil/dbmate/driver/shared"
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func TestDatabaseName(t *testing.T) {
	u, err := url.Parse("ignore://localhost/foo?query")
	require.Nil(t, err)

	name := shared.DatabaseName(u)
	require.Equal(t, "foo", name)
}

func TestDatabaseName_Empty(t *testing.T) {
	u, err := url.Parse("ignore://localhost")
	require.Nil(t, err)

	name := shared.DatabaseName(u)
	require.Equal(t, "", name)
}
