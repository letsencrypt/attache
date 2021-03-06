package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	redis "github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
	logger "github.com/sirupsen/logrus"
)

// CheckHandler wraps an inner redis client and provides a method for handling a
// health check request from Consul. It's exported for use with with a request
// router.
type CheckHandler struct {
	redis.Client
}

// StateOK handles health checks from Consul. A 200 response from this handler
// means that, from this Redis Cluster node's perspective, the Redis Cluster
// State is OK and Consul can begin advertising this node as part of the Redis
// Cluster in the Service Catalog.
func (h *CheckHandler) StateOk(w http.ResponseWriter, r *http.Request) {
	clusterInfo, err := h.GetClusterInfo()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to connect to node %q: %s", h.NodeAddr, err)))
	} else if clusterInfo.State == "ok" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(clusterInfo.State))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(clusterInfo.State))
	}
}

func main() {
	checkServAddr := flag.String("check-serv-addr", "", "address this utility should listen on (e.g. 127.0.0.1:8080)")
	shutdownGrace := flag.Duration("shutdown-grace", time.Second*5, "duration to wait before shutting down (e.g. '1s')")

	var redisOpts config.RedisOpts
	flag.StringVar(&redisOpts.NodeAddr, "redis-node-addr", "", "redis-server listening address, (required)")
	flag.StringVar(&redisOpts.Username, "redis-auth-username", "", "redis-server username, (required)")
	flag.StringVar(&redisOpts.PasswordFile, "redis-auth-password-file", "", "redis-server password file path, (required)")
	flag.StringVar(&redisOpts.CACertFile, "redis-tls-ca-cert", "", "Redis client CA certificate file, (required)")
	flag.StringVar(&redisOpts.CertFile, "redis-tls-cert-file", "", "Redis client certificate file, (required)")
	flag.StringVar(&redisOpts.KeyFile, "redis-tls-key-file", "", "Redis client key file, (required)")
	flag.Parse()

	if *checkServAddr == "" {
		logger.Fatal("Missing required opt 'check-serv-addr'")
	}

	if redisOpts.NodeAddr == "" {
		logger.Fatal("missing required opt: 'redis-node-addr'")
	}

	if redisOpts.Username == "" {
		logger.Fatal("missing required opt: 'redis-auth-username'")
	}

	if redisOpts.PasswordFile == "" {
		logger.Fatal("missing required opt: 'redis-auth-password-file'")
	}

	if redisOpts.CACertFile == "" {
		logger.Fatal("missing required opt: 'redis-tls-ca-cert'")
	}

	if redisOpts.CertFile == "" {
		logger.Fatal("missing required opt: 'redis-tls-cert-file'")
	}

	if redisOpts.KeyFile == "" {
		logger.Fatal("missing required opt: 'redis-tls-key-file'")
	}
	logger.Infof("starting %s", os.Args[0])

	router := mux.NewRouter()
	redisClient, err := redis.New(redisOpts)
	if err != nil {
		logger.Fatalf("redis: %s", err)
	}
	handler := CheckHandler{*redisClient}
	router.HandleFunc("/clusterinfo/state/ok", handler.StateOk)

	server := &http.Server{
		Addr:         *checkServAddr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			logger.Error(err)
		}
	}()

	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	logger.Infof("listening on %s", *checkServAddr)
	<-catchSignals

	ctx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
	defer cancel()
	_ = server.Shutdown(ctx)
	logger.Info("shutting down")
	os.Exit(0)
}
