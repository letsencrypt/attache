locals {
  // await-service-name is the name of the Consul Service that Attache should
  // check for Redis Nodes that are waiting to join a Redis Cluster or waiting
  // to form a new Redis Cluster.
  await-service-name = "redis-cluster-await"

  // dest-service-name is the name of the Consul Service that Attache should
  // check for Redis Nodes that are part of a Redis Cluster that new Redis Nodes
  // should join. 
  dest-service-name = "redis-cluster"

  // primary-count is the count of Redis Shard Primary Nodes that should exist
  // in the resulting Redis Cluster.
  primary-count = 3

  // replica-count is the count of Redis Shard Replica Nodes that should exist
  // in the resulting Redis Cluster.
  replica-count = 3

  // redis-config-template is the Consul Template used to produce the config
  // file for each Redis Node.
  redis-config-template = <<-EOF
    user default off
    masteruser replication-user
    masterauth 435e9c4225f08813ef3af7c725f0d30d263b9cd3
    daemonize no
    # Disables the default TCP port.
    port 0
    bind {{ env "NOMAD_IP_db" }}
    tls-port {{ env "NOMAD_PORT_db" }}
    tls-cert-file {{ env "NOMAD_ALLOC_DIR" }}/data/tls/redis/cert.pem
    tls-key-file {{ env "NOMAD_ALLOC_DIR" }}/data/tls/redis/key.pem
    tls-ca-cert-file {{ env "NOMAD_ALLOC_DIR" }}/data/tls/minica.pem
    tls-cluster yes
    tls-replication yes
    cluster-enabled yes
    cluster-node-timeout 5000
    cluster-config-file {{ env "NOMAD_ALLOC_DIR" }}/data/nodes.conf
    cluster-require-full-coverage no
    appendonly yes
    save 60 1
    maxmemory-policy noeviction
    loglevel warning
    # List of renamed commands comes from:
    # https://www.digitalocean.com/community/tutorials/how-to-secure-your-redis-installation-on-ubuntu-18-04
    rename-command BGREWRITEAOF ""
    rename-command BGSAVE ""
    rename-command CONFIG ""
    rename-command DEBUG ""
    rename-command DEL ""
    rename-command FLUSHALL ""
    rename-command FLUSHDB ""
    rename-command KEYS ""
    rename-command PEXPIRE ""
    rename-command RENAME ""
    rename-command SAVE ""
    rename-command SHUTDOWN ""
    rename-command SPOP ""
    rename-command SREM ""
  EOF
}

job "redis-cluster" {
  datacenters = ["dev-general"]
  type        = "service"
  update {
    max_parallel      = 1
    min_healthy_time  = "5s"
    healthy_deadline  = "5m"
    progress_deadline = "10m"
  }
  group "nodes" {
    count = local.primary-count + local.replica-count
    network {
      // Redis
      port "db" {}
      // Attaché Sidecar
      port "attache" {}
    }
    ephemeral_disk {
      sticky  = true
      migrate = true
      size    = 600
    }
    task "server" {
      service {
        name = local.dest-service-name
        port = "db"
        check {
          name     = "db:tcp-alive"
          type     = "tcp"
          port     = "db"
          interval = "3s"
          timeout  = "2s"
        }
        check {
          name     = "attache:tcp-alive"
          type     = "tcp"
          port     = "attache"
          interval = "3s"
          timeout  = "2s"
        }
        check {
          name     = "attache-check:clusterinfo/state/ok"
          type     = "http"
          port     = "attache"
          path     = "/clusterinfo/state/ok"
          interval = "3s"
          timeout  = "2s"
        }
      }
      resources {
        cpu    = 500
        memory = 512
      }
      driver = "raw_exec"
      config {
        command = "redis-server"
        args    = ["${NOMAD_ALLOC_DIR}/data/redis.conf"]
      }
      template {
        data        = local.redis-config-template
        destination = "${NOMAD_ALLOC_DIR}/data/redis.conf"
        change_mode = "restart"
      }
    }
    task "attache-control" {
      lifecycle {
        hook    = "poststart"
        sidecar = false
      }
      service {
        name = local.await-service-name
        port = "db"
        check {
          name     = "db:tcp-alive"
          type     = "tcp"
          port     = "db"
          interval = "3s"
          timeout  = "2s"
        }
        check {
          name     = "attache:tcp-alive"
          type     = "tcp"
          port     = "attache"
          interval = "3s"
          timeout  = "2s"
        }
      }
      driver = "raw_exec"
      config {
        // command is the path to the built attache-control binary.
        command = "$${HOME}/repos/attache/attache-control"
        args = [
          "-redis-node-addr", "${NOMAD_ADDR_db}",
          "-redis-primary-count", "${local.primary-count}",
          "-redis-replica-count", "${local.replica-count}",
          "-dest-service-name", "${local.dest-service-name}",
          "-await-service-name", "${local.await-service-name}",
        ]
      }
    }
    task "attache-check" {
      lifecycle {
        hook    = "poststart"
        sidecar = true
      }
      driver = "raw_exec"
      config {
        // command is the path to the built attache-check binary.
        command = "$${HOME}/repos/attache/attache-check"
        args = [
          "-redis-node-addr", "${NOMAD_ADDR_db}",
          "-check-serv-addr", "${NOMAD_ADDR_attache}"
        ]
      }
    }
  }
}
