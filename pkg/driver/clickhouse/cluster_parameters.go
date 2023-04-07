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
)

type ClusterParameters struct {
	OnCluster bool
	ZooPath string
	ClusterMacro string
	ReplicaMacro string
}

func ClusterParametersFromURL(u *url.URL) *ClusterParameters {
	onCluster := extractOnCluster(u)
	clusterMacro := extractClusterMacro(u)
	replicaMacro := extractReplicaMacro(u)
	zookeeperPath := extractZookeeperPath(u)

	r := &ClusterParameters{
		OnCluster: onCluster,
		ZooPath: zookeeperPath,
		ClusterMacro: clusterMacro,
		ReplicaMacro: replicaMacro,
	}

	v := u.Query()
	v.Del(OnClusterQueryParam)
	v.Del(ClusterMacroQueryParam)
	v.Del(ReplicaMacroQueryParam)
	v.Del(ZooPathQueryParam)

	return r
}

func extractOnCluster(u *url.URL) bool {
	v := u.Query()
	hasOnCluster := v.Has(OnClusterQueryParam)
	onClusterValue := v.Get(OnClusterQueryParam)
	onCluster := hasOnCluster && (onClusterValue == "" || onClusterValue == "true")
	return onCluster
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

func extractZookeeperPath(u *url.URL) string {
	v := u.Query()
	clusterMacro := extractClusterMacro(u)
	zookeeperPath := v.Get(ZooPathQueryParam)
	if zookeeperPath == "" {
		zookeeperPath = fmt.Sprintf("/clickhouse/tables/%s/{table}", clusterMacro)
	}
	return zookeeperPath
}
