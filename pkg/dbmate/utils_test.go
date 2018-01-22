package dbmate

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatabaseName(t *testing.T) {
	u, err := url.Parse("ignore://localhost/foo?query")
	require.Nil(t, err)

	name := databaseName(u)
	require.Equal(t, "foo", name)
}

func TestDatabaseName_Empty(t *testing.T) {
	u, err := url.Parse("ignore://localhost")
	require.Nil(t, err)

	name := databaseName(u)
	require.Equal(t, "", name)
}

func TestTrimLeadingSQLComments(t *testing.T) {
	in := "--\n" +
		"-- foo\n\n" +
		"-- bar\n\n" +
		"real stuff\n" +
		"-- end\n"
	out, err := trimLeadingSQLComments([]byte(in))
	require.Nil(t, err)
	require.Equal(t, "real stuff\n-- end\n", string(out))
}
