variable "redis-username" {
  type = string
}

variable "redis-password" {
  type = string
}

variable "redis-tls-cacert" {
  type = string
}

variable "redis-tls-cert" {
  type = string
}

variable "redis-tls-key" {
  type = string
}

variable "attache-redis-tls-cert" {
  type = string
}

variable "attache-redis-tls-key" {
  type = string
}

variable "redis-config-template" {
  type = string
}

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
          "-redis-username", "${var.redis-username}",
          "-redis-password-file", "${NOMAD_ALLOC_DIR}/data/password.txt",
          "-redis-tls-enable",
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
          "-redis-username", "${var.redis-username}",
          "-redis-password-file", "${NOMAD_ALLOC_DIR}/data/password.txt",
          "-redis-tls-enable",
          "-redis-tls-ca-cert", "${NOMAD_ALLOC_DIR}/data/redis-tls/ca-cert.pem",
          "-redis-tls-cert-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/cert.pem",
          "-redis-tls-key-file", "${NOMAD_ALLOC_DIR}/data/attache-tls/key.pem"
        ]
      }
    }
  }
}
