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
	redisClient "github.com/letsencrypt/attache/src/redis/client"
	"github.com/letsencrypt/attache/src/redis/config"
	logger "github.com/sirupsen/logrus"
)

// CheckHandler is a wrapper around an inner redis.Client.
type CheckHandler struct {
	redisClient.Client
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
	var redisConf config.RedisConfig
	checkServAddr := flag.String("check-serv-addr", "", "address this utility should listen on (e.g. 127.0.0.1:8080)")
	shutdownGrace := flag.Duration("shutdown-grace", time.Second*5, "duration to wait before shutting down (e.g. '1s')")
	flag.StringVar(&redisConf.NodeAddr, "redis-node-addr", "", "redis-server listening address")

	logger.Infof("starting %s", os.Args[0])
	flag.Parse()

	if *checkServAddr == "" {
		logger.Fatal("Missing required opt 'check-serv-addr'")
	}

	if redisConf.NodeAddr == "" {
		logger.Fatal("Missing required opt 'redis-node-addr'")
	}

	router := mux.NewRouter()
	redisClient, err := redisClient.New(redisConf)
	if err != nil {
		logger.Fatalf("redis: %s")
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
