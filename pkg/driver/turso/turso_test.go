package turso

import (
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbtest"
	"github.com/stretchr/testify/require"
)

func TestConnectionString(t *testing.T) {
	t.Run("substitutes schema", func(t *testing.T) {
		u := dbtest.MustParseURL(t, "turso://example-database-dbmate.turso.io?authToken=fakeToken")
		require.Equal(t, "libsql://example-database-dbmate.turso.io?authToken=fakeToken", connectionString(u))
	})
}
