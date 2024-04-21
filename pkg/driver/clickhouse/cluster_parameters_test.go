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
	}{
		// zoo_path not supplied
		{"clickhouse://myhost:9000", "/clickhouse/tables/{cluster}/{table}"},
		// zoo_path supplied
		{"clickhouse://myhost:9000?zoo_path=/zk/path/tables", "/zk/path/tables"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			u := dbtest.MustParseURL(t, c.input)

			actual := extractZookeeperPath(u)
			require.Equal(t, c.expected, actual)
		})
	}
}
