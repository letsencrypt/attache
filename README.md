# Attaché
A sidecar that allows for effortless scaling of Redis Clusters in Hashicorp
Nomad.

## Features
- **Cluster Initialization:** the Attaché for each newly created primary node
  - Checks the Consul Service Catalog for it's configured `intendedCluster`
    - If the `intendedCluster` doesn't exist, attempts to aquire a distributed
      lock between all nodes in `awaitMeet` tagged `primary`, and then perform a
      redis-cli --create-cluster
    - If the `intendedCluster` does exist, goes idle
- **Primary Scaling:** the Attaché for each shard primary detects when new nodes
  (tagged `primary`) are added to the configured `awaitMeet` Consul Service,
  joins them as a new primary, and performs a shardslot rebalance
- **Replica Scaling:** the Attaché for each shard primary detects when new
  secondary nodes are added to the configured `awaitMeet` Consul Service
