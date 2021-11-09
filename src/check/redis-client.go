package check

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-redis/redis"
	"gopkg.in/yaml.v3"
)

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

type RedisCheckHandler struct {
	nodeAddr string
	client   *redis.Client
}

func NewRedisCheckHandler(redisNodeAddr, redisNodePass string) *RedisCheckHandler {
	return &RedisCheckHandler{
		nodeAddr: redisNodeAddr,
		client: redis.NewClient(
			&redis.Options{
				Addr:     redisNodeAddr,
				Password: redisNodePass,
			},
		),
	}
}

func (h *RedisCheckHandler) getClusterInfo() (*RedisClusterInfo, error) {
	info, err := h.client.ClusterInfo().Result()
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

func (h *RedisCheckHandler) StateOk(w http.ResponseWriter, r *http.Request) {
	clusterInfo, err := h.getClusterInfo()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Unable to connect to node %q: %s", h.nodeAddr, err)))
	} else if clusterInfo.State == "ok" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(clusterInfo.State))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(clusterInfo.State))
	}
}

func (h *RedisCheckHandler) StateNew(w http.ResponseWriter, r *http.Request) {
	var infoMatchingNewNodes = RedisClusterInfo{"fail", 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}
	clusterInfo, err := h.getClusterInfo()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Unable to connect to node %q: %s", h.nodeAddr, err)))
	} else if *clusterInfo == infoMatchingNewNodes {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("true"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("false"))
	}
}
