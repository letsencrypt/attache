package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

type RedisHandler struct {
	nodeAddr string
	client   *redis.Client
}

func NewRedisHandler(redisNodeAddr, redisNodePass string) *RedisHandler {
	return &RedisHandler{
		nodeAddr: redisNodeAddr,
		client: redis.NewClient(
			&redis.Options{
				Addr:     redisNodeAddr,
				Password: redisNodePass,
			},
		),
	}
}

type RedisClusterInfo struct {
	State                 string `yaml:"cluster_state"`
	SlotsAssigned         int    `yaml:"cluster_slots_assigned"`
	SlotsOk               int    `yaml:"cluster_slots_ok"`
	SlotsPfail            int    `yaml:"cluster_slots_pfail"`
	SlotsFail             int    `yaml:"cluster_slots_fail"`
	KnownNodes            int    `yaml:"cluster_known_nodes"`
	Size                  int    `yaml:"cluster_size"`
	CurrentEpoch          int    `yaml:"cluster_current_epoch"`
	MyEpoch               int    `yaml:"cluster_my_epoch"`
	StatsMessagesSent     int    `yaml:"cluster_stats_messages_sent"`
	StatsMessagesReceived int    `yaml:"cluster_stats_messages_received"`
}

func (h *RedisHandler) getClusterInfo() (*RedisClusterInfo, error) {
	info, err := h.client.ClusterInfo(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	info = strings.ReplaceAll(info, ":", ": ")

	var clusterInfo RedisClusterInfo
	err = yaml.Unmarshal([]byte(info), &clusterInfo)
	if err != nil {
		return nil, err
	}

	return &clusterInfo, nil
}

func (h *RedisHandler) clusterStateOk(w http.ResponseWriter, r *http.Request) {
	clusterInfo, err := h.getClusterInfo()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Unable to connect to node %q: %s", h.nodeAddr, err)))

	} else if clusterInfo.State != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(clusterInfo.State))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(clusterInfo.State))
	}
}

func (h *RedisHandler) clusterSlotsAssignedGt0(w http.ResponseWriter, r *http.Request) {
	status, err := h.client.ClusterInfo(context.Background()).Result()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Unable to connect to node %q", h.nodeAddr)))
	} else {
		log.Println(status)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Node %q is role %q", h.nodeAddr, status)))
	}
}

func (h *RedisHandler) clusterKnownNodesGt1(w http.ResponseWriter, r *http.Request) {
	status, err := h.client.ClusterInfo(context.Background()).Result()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf("Unable to connect to node %q", h.nodeAddr)))
	} else {
		log.Println(status)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Node %q is role %q", h.nodeAddr, status)))
	}
}

func main() {
	checkServAddr := flag.String("check-serv-addr", "", "Address the check server should listen on (example: '0.0.0.0:8080')")
	redisNodeAddr := flag.String("redis-node-addr", "", "Address of the Redis node to be monitored (example: '127.0.0.1:6049')")
	shutdownWait := flag.Duration("shutdown-wait", time.Second*15, "duration to wait for existing connections to finish (example: '1s', '1m', '1h')")
	flag.Parse()

	if *checkServAddr == "" {
		log.Fatalln("Missing required opt 'check-serv-addr'")
	}

	if *redisNodeAddr == "" {
		log.Fatalln("Missing required opt 'redis-node-addr'")
	}

	router := mux.NewRouter()
	handler := NewRedisHandler(*redisNodeAddr, "")
	router.HandleFunc("/redis/clusterinfo/ok", handler.clusterStateOk)

	server := &http.Server{
		Addr:         *checkServAddr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	<-catchSignals

	ctx, cancel := context.WithTimeout(context.Background(), *shutdownWait)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("shutting down")
	os.Exit(0)
}
