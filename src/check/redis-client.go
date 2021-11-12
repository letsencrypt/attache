package check

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-redis/redis"
	"gopkg.in/yaml.v3"
)

type Client struct {
	nodeAddr string
	client   *redis.Client
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

func (h *Client) GetPrimaryWithLeastReplicas() (string, string, error) {
	nodes, err := h.getClusterNodes()
	if err != nil {
		return "", "", err
	}

	counts := make(map[redisClusterNode]int)
	for _, n := range nodes {
		_, ok := counts[n]
		if !ok {
			counts[n] = 0
		}
		if n.replicaOf != "-" {
			counts[n] += 1
		}
	}

	sortedCounts := make([]redisClusterNode, 0, len(counts))
	for key := range counts {
		sortedCounts = append(sortedCounts, key)
	}
	sort.Slice(
		sortedCounts,
		func(i, j int) bool {
			return counts[sortedCounts[i]] < counts[sortedCounts[j]]
		},
	)
	return sortedCounts[0].nodeAddr, sortedCounts[0].nodeID, nil
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

type redisClusterNode struct {
	nodeID    string
	nodeAddr  string
	role      string
	replicaOf string
}

func (h *Client) getClusterNodes() ([]redisClusterNode, error) {
	output, err := h.client.ClusterNodes().Result()
	if err != nil {
		return nil, err
	}

	// Replica nodes are missing the slots column, so we can just add one to
	// make the number of values per row equal and avoid ignoring all
	// `csv.ErrFieldCount`.
	output = strings.ReplaceAll(output, "connected\n", "connected 0-0\n")

	// Replacing myself,<role> to make `role` more consistent.
	output = strings.ReplaceAll(output, "myself,master", "master")
	output = strings.ReplaceAll(output, "myself,slave", "slave")

	var nodes []redisClusterNode
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

		nodes = append(
			nodes,
			redisClusterNode{record[0], record[1], record[2], record[3]},
		)
	}

	if len(nodes) == 0 {
		return nil, errors.New("no nodes found in 'cluster nodes' output")
	}
	return nodes, nil
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
