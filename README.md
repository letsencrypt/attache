# Attaché

A sidecar that allows for effortless scaling of Redis Clusters in Hashicorp
Nomad.

## Features
- Cluster Create
- New Primary Node Introduction and Shardslot Rebalance
- New Replica Node Introduction to the Primary with the least Replicas

## How to build

  go build ./cmd/check/attache-check.go && go build ./cmd/control/attache-control.go