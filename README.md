# Attaché
A sidecar that allows for effortless scaling of Redis Clusters using Hashicorp
Nomad and Consul.

#### Features
- Create a new cluster when no cluster is present
- Add new primary node and perform a shard slot rebalance
- Add new replica node to the primary node with the least replicas
- Full support for Redis mTLS and ACL Auth
- Full support for Consul mTLS and ACL Tokens

#### To Do
- [x] Redis ACL
- [x] Redis Password
- [x] Redis mTLS
- [ ] Drain, failover, and FORGET an existing primary node
- [ ] Remove and FORGET an existing replica node

### `attache-check`
A sidecar that servers an HTTP API that allows Consul to track the health of
Redis Cluster Nodes, route new nodes to the Await (introduction) Consul Service
for their Redis Cluster, then migrate them to the Destination Consul Service
once they've joined a cluster.

#### Usage
```shell
$ attache-check -help
Usage of attache-check:
  -check-serv-addr string
    	address this utility should listen on (e.g. 127.0.0.1:8080)
  -redis-auth-password-file string
    	redis-server password file path, (required)
  -redis-auth-username string
    	redis-server username, (required)
  -redis-node-addr string
    	redis-server listening address, (required)
  -redis-tls-ca-cert string
    	Redis client CA certificate file, (required)
  -redis-tls-cert-file string
    	Redis client certificate file, (required)
  -redis-tls-key-file string
    	Redis client key file, (required)
  -shutdown-grace duration
    	duration to wait before shutting down (e.g. '1s') (default 5s)
```

### `attache-control`
An ephemeral sidecar that acts as an agent for each Redis node when it's
started. If a node's `node info` reflects that of a new node, this agent will
attempt to introduce it to an existing Redis Cluster, if it exists, else it will
attempt to orchestrate the create a new Redis Cluster if there are enough new
Redis nodes (in the Await Consul Service) to do so.

#### Usage
```shell
$ ./attache-control -help
Usage of ./attache-control:
  -attempt-interval duration
    	Duration to wait between attempts to join or create a cluster (default 3s)
  -attempt-limit int
    	Number of times to attempt joining or creating a cluster before exiting (default 20)
  -await-service-name string
    	Consul Service for newly created Redis Cluster Nodes, (required)
  -consul-acl-token string
    	Consul client ACL token
  -consul-addr string
    	Consul client address (default "127.0.0.1:8501")
  -consul-dc string
    	Consul client datacenter (default "dev-general")
  -consul-tls-ca-cert string
    	Consul client CA certificate file
  -consul-tls-cert string
    	Consul client certificate file
  -consul-tls-enable
    	Enable mTLS for the Consul client
  -consul-tls-key string
    	Consul client key file
  -dest-service-name string
    	Consul Service for healthy Redis Cluster Nodes, (required)
  -lock-kv-path string
    	Consul KV path used as a distributed lock for operations (default "service/attache/leader")
  -log-level string
    	Set the log level (default "info")
  -redis-auth-password-file string
    	Redis password file path, (required)
  -redis-auth-username string
    	Redis username, (required)
  -redis-node-addr string
    	redis-server listening address, (required)
  -redis-tls-ca-cert string
    	Redis client CA certificate file, (required)
  -redis-tls-cert-file string
    	Redis client certificate file, (required)
  -redis-tls-key-file string
    	Redis client key file, (required)
```

### Running the Example Nomad Job
Note: these steps assume that you have the `nomad`, `consul`, and `terraform`
binaries installed on your machine and that they exist in your `PATH`.

Build the attache-control and attache-check binaries:
```shell
$ go build -o attache-check ./cmd/attache-check/main.go && go build -o attache-control ./cmd/attache-control/main.go ./cmd/attache-control/config.go
```

In another shell, start the Consul server in `dev` mode:
```shell
$ consul agent -dev -datacenter dev-general -log-level ERROR
```

In another shell, start the Nomad server in `dev` mode:
```shell
$ sudo nomad agent -dev -bind 0.0.0.0 -log-level ERROR -dc dev-general
```

Start a Nomad job deployment using Terraform:
```shell
cd example
terraform init
terraform plan
terraform apply
```

Open the Nomad UI: http://localhost:4646/ui to view information about the Redis
Cluster deployment

Open the Consul UI: http://localhost:8501/ui to view health check information
for the Redis Cluster

### Useful Commands

#### Purge Nomad Job
This is useful for stopping and garbage collecting a job in Nomad immediately.
```shell
nomad job stop -purge "<jobname>"
```

#### Count Primary Nodes
```shell
redis-cli -p <tls-port> --tls --cert ./example/tls/redis/cert.pem --key ./example/tls/redis/key.pem --cacert ./example/tls/ca-cert.pem --user replication-user --pass <redis-password> cluster nodes | grep master | wc -l
```

#### Count Replica Nodes
```shell
redis-cli -p <tls-port> --tls --cert ./example/tls/redis/cert.pem --key ./example/tls/redis/key.pem --cacert ./example/tls/ca-cert.pem --user replication-user --pass <redis-password> cluster nodes | grep slave | wc -l
```
