// await-service-name is the name of the Consul Service that Attache should
// check for Redis Nodes that are waiting to join a Redis Cluster or waiting to
// form a new Redis Cluster.
variable "await-service-name" {
  type = string
}

// dest-service-name is the name of the Consul Service that Attache should check
// for Redis Nodes that are part of a Redis Cluster that new Redis Nodes should
// join.
variable "dest-service-name" {
  type = string
}

// primary-count is the count of Redis Shard Primary Nodes that should exist in
// the resulting Redis Cluster.
variable "primary-count" {
  type = number
}

// replica-count is the count of Redis Shard Replica Nodes that should exist in
// the resulting Redis Cluster.
variable "replica-count" {
  type = number
}

// redis-username is the username that will be set as `masteruser` for each
// Redis Cluster Node and used each time Attaché connects to a Redis Cluster
// Node.
variable "redis-username" {
  type = string
}

// redis-password is the password that will be set as `masterauth` for each
// Redis Cluster Node and used each time Attaché connects to a Redis Cluster
// Node.
variable "redis-password" {
  type = string
}

// redis-tls-cacert is the contents of the CA cert file, in PEM format, used for
// mutal TLS authentication between Redis Server and Attaché.
variable "redis-tls-cacert" {
  type = string
}

// redis-tls-cert is the contents of the cert file, in PEM format, used for
// mutal TLS authentication between Redis Server and Attaché.
variable "redis-tls-cert" {
  type = string
}

// redis-tls-key is the contents of the key file, in PEM format, used for mutal
// TLS authentication between Redis Server and Attaché.
variable "redis-tls-key" {
  type = string
}

// redis-config-template is template used to create the configuration file used
// loaded by each Redis Cluster Node
variable "redis-config-template" {
  type = string
}

// attache-redis-tls-cert is the contents of the cert file, in PEM format, used
// for mutal TLS authentication between Attaché and the Redis Server.
variable "attache-redis-tls-cert" {
  type = string
}

// attache-redis-tls-key is the contents of the key file, in PEM format, used
// for mutal TLS authentication between Attaché and the Redis Server.
variable "attache-redis-tls-key" {
  type = string
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
    count = var.primary-count + var.replica-count
    network {
      // Redis
      port "db" {}
      // Attaché Sidecar
      port "attache" {}
    }
    ephemeral_disk {
      sticky  = true
      migrate = true
    }
    task "server" {
      service {
        name = var.dest-service-name
        port = "db"
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
      driver = "raw_exec"
      config {
        command = "redis-server"
        args    = ["${NOMAD_ALLOC_DIR}/data/redis.conf"]
      }
      env {
        redis-password = "${var.redis-password}"
      }
      template {
        data        = var.redis-config-template
        destination = "${NOMAD_ALLOC_DIR}/data/redis.conf"
        change_mode = "restart"
      }
      template {
        data        = var.redis-password
        destination = "${NOMAD_ALLOC_DIR}/data/password.txt"
        change_mode = "restart"
      }
      template {
        data        = var.redis-tls-cacert
        destination = "${NOMAD_ALLOC_DIR}/data/redis-tls/ca-cert.pem"
        change_mode = "restart"
      }
      template {
        data        = var.redis-tls-cert
        destination = "${NOMAD_ALLOC_DIR}/data/redis-tls/cert.pem"
        change_mode = "restart"
      }
      template {
        data        = var.redis-tls-key
        destination = "${NOMAD_ALLOC_DIR}/data/redis-tls/key.pem"
        change_mode = "restart"
      }
      template {
        data        = var.attache-redis-tls-cert
        destination = "${NOMAD_ALLOC_DIR}/data/attache-tls/cert.pem"
        change_mode = "restart"
      }
      template {
        data        = var.attache-redis-tls-key
        destination = "${NOMAD_ALLOC_DIR}/data/attache-tls/key.pem"
        change_mode = "restart"
      }
    }
    task "attache-control" {
      lifecycle {
        hook    = "poststart"
        sidecar = false
      }
      service {
        name = var.await-service-name
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
          "-redis-primary-count", "${var.primary-count}",
          "-redis-replica-count", "${var.replica-count}",
          "-dest-service-name", "${var.dest-service-name}",
          "-await-service-name", "${var.await-service-name}",
          "-redis-auth-username", "${var.redis-username}",
          "-redis-auth-password-file", "${NOMAD_ALLOC_DIR}/data/password.txt",
          "-redis-tls-ca-cert", "${NOMAD_ALLOC_DIR}/data/redis-tls/ca-cert.pem",
          "-redis-tls-cert-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/cert.pem",
          "-redis-tls-key-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/key.pem"
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
          "-check-serv-addr", "${NOMAD_ADDR_attache}",
          "-redis-auth-username", "${var.redis-username}",
          "-redis-auth-password-file", "${NOMAD_ALLOC_DIR}/data/password.txt",
          "-redis-tls-ca-cert", "${NOMAD_ALLOC_DIR}/data/redis-tls/ca-cert.pem",
          "-redis-tls-cert-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/cert.pem",
          "-redis-tls-key-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/key.pem"
        ]
      }
    }
  }
}
