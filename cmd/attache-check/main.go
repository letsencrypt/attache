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

// CheckHandler is a wraps an inner redis client with some methods for handling
// health check requests.
type CheckHandler struct {
	redis.Client
}

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
	flag.StringVar(&redisOpts.NodeAddr, "redis-node-addr", "", "redis-server listening address")
	flag.BoolVar(&redisOpts.EnableAuth, "redis-auth-enable", false, "Enable auth for the Redis client and redis-cli")
	flag.StringVar(&redisOpts.Username, "redis-auth-username", "", "redis-server username")
	flag.StringVar(&redisOpts.PasswordFile, "redis-auth-password-file", "", "redis-server password file path")
	flag.BoolVar(&redisOpts.EnableTLS, "redis-tls-enable", false, "Enable mTLS for the Redis client")
	flag.StringVar(&redisOpts.CACertFile, "redis-tls-ca-cert", "", "Redis client CA certificate file")
	flag.StringVar(&redisOpts.CertFile, "redis-tls-cert-file", "", "Redis client certificate file")
	flag.StringVar(&redisOpts.KeyFile, "redis-tls-key-file", "", "Redis client key file")
	flag.Parse()

	if *checkServAddr == "" {
		logger.Fatal("Missing required opt 'check-serv-addr'")
	}

	err := redisOpts.Validate()
	if err != nil {
		logger.Fatal(err)
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
