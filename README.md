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
- [] Drain, failover, and FORGET an existing primary node
- [] Remove and FORGET an existing replica node

### `attache-check`
A sidecar that servers an HTTP API that allows Consul to track the health of
Redis Cluster Nodes, route new nodes to the Await (introduction) Consul Service
for their Redis Cluster, then migrate them to the Destination Consul Service
once they've joined a cluster.

#### Usage
```shell
Usage of ./attache-check:
  -check-serv-addr string
    	address this utility should listen on (e.g. 127.0.0.1:8080)
  -redis-auth-enable
    	Enable auth for the Redis client and redis-cli
  -redis-auth-password-file string
    	redis-server password file path
  -redis-auth-username string
    	redis-server username
  -redis-node-addr string
    	redis-server listening address
  -redis-tls-ca-cert string
    	Redis client CA certificate file
  -redis-tls-cert-file string
    	Redis client certificate file
  -redis-tls-enable
    	Enable mTLS for the Redis client
  -redis-tls-key-file string
    	Redis client key file
  -shutdown-grace duration
    	duration to wait before shutting down (e.g. '1s') (default 5s)
```

### `attache-control`
An ephemeral sidecar that acts as an agent for each Redis node when it's
started. If a node's `node info` reflects that of a new node, this agent will
attempt to introduce it to an existing Redis Cluster, if it exists, else it will
attempt to orchestrate the create a new Redis Cluster if there are enough new
Redis Nodes (in the Await Consul Service) to do so.

#### Usage
```shell
$ attache-control -help
Usage of ./attache-control:
  -attempt-interval duration
    	Duration to wait between attempts to join or create a cluster (default 3s)
  -attempt-limit int
    	Number of times to attempt joining or creating a cluster before exiting (default 20)
  -await-service-name string
    	Consul Service for any newly created Redis Cluster Nodes
  -consul-acl-token string
    	Consul client ACL token
  -consul-addr string
    	Consul client address (default "127.0.0.1:8500")
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
    	Consul Service for any existing Redis Cluster Nodes
  -lock-kv-path string
    	Consul KV path used as a distributed lock for operations (default "service/attache/leader")
  -log-level string
    	Set the log level (default "info")
  -redis-auth-enable
    	Enable auth for the Redis client and redis-cli
  -redis-auth-password-file string
    	Redis password file path
  -redis-auth-username string
    	Redis username
  -redis-node-addr string
    	redis-server listening address
  -redis-primary-count int
    	Total number of expected Redis shard primary nodes
  -redis-replica-count int
    	Total number of expected Redis shard replica nodes
  -redis-tls-ca-cert string
    	Redis client CA certificate file
  -redis-tls-cert-file string
    	Redis client certificate file
  -redis-tls-enable
    	Enable mTLS for the Redis client
  -redis-tls-key-file string
    	Redis client key file
```

### Running the Example Nomad Job
Note: these steps assume that you have the `nomad` and `consul` binaries installed
on your machine and that they exist in your `PATH`.

Build the attache-control and attache-check binaries:
```shell
$ go build -o attache-check ./cmd/attache-check/main.go && go build -o attache-control ./cmd/attache-control/main.go ./cmd/attache-control/config.go
```

Start the Consul server in `dev` mode:
```shell
$ consul agent -dev -datacenter dev-general -log-level ERROR
```

Start the Nomad server in `dev` mode:
```shell
$ sudo nomad agent -dev -bind 0.0.0.0 -log-level ERROR -dc dev-general
```

Start a Nomad job deployment:
```shell
$ nomad job run -verbose -var-file=./example/vars-file.hcl ./example/job-specification.hcl
```

Open the Nomad UI: http://localhost:4646/ui

Open the Consul UI: http://localhost:8500/ui