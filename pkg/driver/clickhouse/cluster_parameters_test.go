package clickhouse

import (
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbtest"

	"github.com/stretchr/testify/require"
)

func TestOnCluster(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		// param not supplied
		{"clickhouse://myhost:9000", false},
		// empty on_cluster parameter
		{"clickhouse://myhost:9000?on_cluster", true},
		// true on_cluster parameter
		{"clickhouse://myhost:9000?on_cluster=true", true},
		// any other value on_cluster parameter
		{"clickhouse://myhost:9000?on_cluster=falsy", false},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual := extractOnCluster(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestClusterMacro(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// cluster_macro not supplied
		{"clickhouse://myhost:9000", "{cluster}"},
		// cluster_macro supplied
		{"clickhouse://myhost:9000?cluster_macro={cluster2}", "{cluster2}"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual := extractClusterMacro(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestReplicaMacro(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// replica_macro not supplied
		{"clickhouse://myhost:9000", "{replica}"},
		// replica_macro supplied
		{"clickhouse://myhost:9000?replica_macro={replica2}", "{replica2}"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual := extractReplicaMacro(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestZookeeperPath(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		set      bool
	}{
		// zoo_path not supplied
		{"clickhouse://myhost:9000", "/clickhouse/tables/{cluster}/{table}", false},
		// zoo_path present but empty is treated as unset
		{"clickhouse://myhost:9000?zoo_path=", "/clickhouse/tables/{cluster}/{table}", false},
		// zoo_path supplied
		{"clickhouse://myhost:9000?zoo_path=/zk/path/tables", "/zk/path/tables", true},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual, set := extractZookeeperPath(u)
			require.Equal(t, c.expected, actual)
			require.Equal(t, c.set, set)
		})
	}
}

func TestReplicated(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"clickhouse://myhost:9000", false},
		{"clickhouse://myhost:9000?replicated", true},
		{"clickhouse://myhost:9000?replicated=true", true},
		{"clickhouse://myhost:9000?replicated=falsy", false},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual := extractReplicated(u)
			require.Equal(t, c.expected, actual)
		})
	}
}

func TestOnClusterAndReplicatedMutuallyExclusive(t *testing.T) {
	drv := testClickHouseDriverURL(t, dbtest.MustParseURL(t,
		"clickhouse://myhost:9000/dbmate_test?on_cluster&replicated"))

	err := drv.CreateDatabase()
	require.EqualError(t, err, "clickhouse: on_cluster and replicated are mutually exclusive")

	err = drv.CreateMigrationsTable(nil)
	require.EqualError(t, err, "clickhouse: on_cluster and replicated are mutually exclusive")
}

func TestCreateDatabaseRefusesReplicated(t *testing.T) {
	drv := testClickHouseDriverURL(t, dbtest.MustParseURL(t,
		"clickhouse://myhost:9000/dbmate_test?replicated"))

	err := drv.CreateDatabase()
	require.EqualError(t, err,
		"clickhouse: replicated requires a database already created with ENGINE = Replicated(...) (or Shared on ClickHouse Cloud); dbmate create cannot create that database")
}
