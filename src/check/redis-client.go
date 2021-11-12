package check

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-redis/redis"
	"gopkg.in/yaml.v3"
)

type Client struct {
	nodeAddr string
	client   *redis.Client
}

type redisClusterNodes []struct {
	nodeID   string
	nodeAddr string
	role     string
}

func (h *Client) GetClusterNodes() (*redisClusterNodes, error) {
	output, err := h.client.ClusterNodes().Result()
	if err != nil {
		return nil, err
	}
	output = strings.ReplaceAll(output, "connected\n", "connected 0-0\n")
	output = strings.ReplaceAll(output, "myself,master", "master")
	output = strings.ReplaceAll(output, "myself,slave", "slave")

	var nodes *redisClusterNodes
	reader := csv.NewReader(strings.NewReader(output))
	reader.Comma = ' '
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		fmt.Println(record[0], record[1], record[2])
	}
	return nodes, nil
}

func (h *Client) StateOkHandler(w http.ResponseWriter, r *http.Request) {
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

func (h *Client) StateNewHandler(w http.ResponseWriter, r *http.Request) {
	var infoMatchingNewNodes = redisClusterInfo{"fail", 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}
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

type redisClusterInfo struct {
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

func (h *Client) getClusterInfo() (*redisClusterInfo, error) {
	info, err := h.client.ClusterInfo().Result()
	if err != nil {
		return nil, err
	}

	var clusterInfo redisClusterInfo
	err = yaml.Unmarshal([]byte(strings.ReplaceAll(info, ":", ": ")), &clusterInfo)
	if err != nil {
		return nil, err
	}

	return &clusterInfo, nil
}

func NewCheckClient(redisNodeAddr, redisNodePass string) *Client {
	return &Client{
		nodeAddr: redisNodeAddr,
		client: redis.NewClient(
			&redis.Options{
				Addr:     redisNodeAddr,
				Password: redisNodePass,
			},
		),
	}
}
