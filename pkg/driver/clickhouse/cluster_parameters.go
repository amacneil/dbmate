package clickhouse

import (
	"fmt"
	"net/url"
)

const (
	OnClusterQueryParam    = "on_cluster"
	ZooPathQueryParam      = "zoo_path"
	ClusterMacroQueryParam = "cluster_macro"
	ReplicaMacroQueryParam = "replica_macro"
	ReplicatedQueryParam   = "replicated"
)

type ClusterParameters struct {
	OnCluster    bool
	ZooPath      string
	ZooPathSet   bool
	ClusterMacro string
	ReplicaMacro string
	Replicated   bool
}

func ClearClusterParametersFromURL(u *url.URL) *url.URL {
	q := u.Query()
	q.Del(OnClusterQueryParam)
	q.Del(ClusterMacroQueryParam)
	q.Del(ReplicaMacroQueryParam)
	q.Del(ZooPathQueryParam)
	q.Del(ReplicatedQueryParam)
	u.RawQuery = q.Encode()

	return u
}

func ExtractClusterParametersFromURL(u *url.URL) *ClusterParameters {
	onCluster := extractOnCluster(u)
	clusterMacro := extractClusterMacro(u)
	replicaMacro := extractReplicaMacro(u)
	zookeeperPath, zooPathSet := extractZookeeperPath(u)
	replicated := extractReplicated(u)

	r := &ClusterParameters{
		OnCluster:    onCluster,
		ZooPath:      zookeeperPath,
		ZooPathSet:   zooPathSet,
		ClusterMacro: clusterMacro,
		ReplicaMacro: replicaMacro,
		Replicated:   replicated,
	}

	return r
}

func extractBoolQueryParam(u *url.URL, name string) bool {
	v := u.Query()
	has := v.Has(name)
	value := v.Get(name)
	return has && (value == "" || value == "true")
}

func extractOnCluster(u *url.URL) bool {
	return extractBoolQueryParam(u, OnClusterQueryParam)
}

func extractReplicated(u *url.URL) bool {
	return extractBoolQueryParam(u, ReplicatedQueryParam)
}

func extractClusterMacro(u *url.URL) string {
	v := u.Query()
	clusterMacro := v.Get(ClusterMacroQueryParam)
	if clusterMacro == "" {
		clusterMacro = "{cluster}"
	}
	return clusterMacro
}

func extractReplicaMacro(u *url.URL) string {
	v := u.Query()
	replicaMacro := v.Get(ReplicaMacroQueryParam)
	if replicaMacro == "" {
		replicaMacro = "{replica}"
	}
	return replicaMacro
}

func extractZookeeperPath(u *url.URL) (string, bool) {
	v := u.Query()
	zookeeperPath := v.Get(ZooPathQueryParam)
	if zookeeperPath == "" {
		clusterMacro := extractClusterMacro(u)
		return fmt.Sprintf("/clickhouse/tables/%s/{table}", clusterMacro), false
	}
	return zookeeperPath, true
}
