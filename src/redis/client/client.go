package client

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/letsencrypt/attache/src/redis/config"
)

// Client is a wrapper around an inner go-redis client.
type Client struct {
	// NodeAddr is the address of the Redis node (e.g. 127.0.0.1:7070).
	NodeAddr string
	// Client is is a go-redis Client.
	Client *redis.Client
}

func (h *Client) StateNewCheck() (bool, error) {
	var infoMatchingNewNodes = clusterInfo{"fail", 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}
	clusterInfo, err := h.GetClusterInfo()
	if err != nil {
		return false, err
	} else if *clusterInfo == infoMatchingNewNodes {
		return true, nil
	} else {
		return false, nil
	}
}

func (h *Client) GetPrimaryWithLeastReplicas() (string, string, error) {
	nodes, err := h.getClusterNodes(true, false, false)
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

func (h *Client) GetPrimaryNodes() ([]redisClusterNode, error) {
	nodes, err := h.getClusterNodes(true, true, false)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (h *Client) GetReplicaNodes() ([]redisClusterNode, error) {
	nodes, err := h.getClusterNodes(true, false, true)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

type clusterInfo struct {
	State                 string `name:"cluster_state"`
	SlotsAssigned         int64  `name:"cluster_slots_assigned"`
	SlotsOk               int64  `name:"cluster_slots_ok"`
	SlotsPfail            int64  `name:"cluster_slots_pfail"`
	SlotsFail             int64  `name:"cluster_slots_fail"`
	KnownNodes            int64  `name:"cluster_known_nodes"`
	Size                  int64  `name:"cluster_size"`
	CurrentEpoch          int64  `name:"cluster_current_epoch"`
	MyEpoch               int64  `name:"cluster_my_epoch"`
	StatsMessagesSent     int64  `name:"cluster_stats_messages_sent"`
	StatsMessagesReceived int64  `name:"cluster_stats_messages_received"`
}

func setClusterInfoField(name string, value string, ci *clusterInfo) error {
	outType := reflect.TypeOf(*ci)
	outValue := reflect.ValueOf(ci).Elem()
	for i := 0; i < outType.NumField(); i++ {
		field := outType.Field(i)
		fieldValue := outValue.Field(i)

		if !fieldValue.IsValid() || !fieldValue.CanSet() {
			continue
		}
		fieldName := field.Tag.Get("name")
		if fieldName != name {
			continue
		}

		switch field.Type.Kind() {
		case reflect.Int64:
			vInt, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("couldn't parse %q, value of %q, as int: %w", value, name, err)
			}
			fieldValue.SetInt(int64(vInt))
			return nil

		case reflect.String:
			fieldValue.SetString(value)
			return nil
		}
	}
	return nil
}

// unmarshalClusterInfo constructs a *clusterInfo by parsing the (INFO style) output
// of the 'cluster info' command as specified in:
// https://redis.io/commands/cluster-info.
func unmarshalClusterInfo(info string) (*clusterInfo, error) {
	var c clusterInfo
	for _, line := range strings.Split(info, "\r\n") {
		// https://redis.io/commands/info#return-value
		if strings.Contains(line, "#") || line == "" {
			continue
		}
		kv := strings.Split(line, ":")
		if len(kv) != 2 {
			return nil, fmt.Errorf("couldn't parse 'cluster info', expected '<key>:<value>', got %q", line)
		}
		err := setClusterInfoField(kv[0], kv[1], &c)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'cluster info': %w", err)
		}
	}
	return &c, nil
}

func (h *Client) GetClusterInfo() (*clusterInfo, error) {
	info, err := h.Client.ClusterInfo(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	return unmarshalClusterInfo(info)
}

type redisClusterNode struct {
	nodeID     string
	nodeAddr   string
	role       string
	replicaOf  string
	connection string
}

func parseClusterNodesResult(connectedOnly, primaryOnly, replicaOnly bool, result string) ([]redisClusterNode, error) {
	// Remove the slots column to make the number of values per row equal and
	// avoid ignoring all `csv.ErrFieldCount`.
	output := strings.Split(result, "\n")
	for i, line := range output {
		output[i] = strings.SplitAfter(line, "connected")[0]
	}
	result = strings.Join(output, "\n")

	// Replacing myself,<role> to make `role` more consistent.
	result = strings.ReplaceAll(result, "myself,master", "master")
	result = strings.ReplaceAll(result, "myself,slave", "slave")
	result = strings.ReplaceAll(result, "master,fail", "master")
	result = strings.ReplaceAll(result, "slave,fail", "slave")

	var nodes []redisClusterNode
	reader := csv.NewReader(strings.NewReader(result))
	reader.Comma = ' '
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		if connectedOnly {
			if record[7] == "disconnected" {
				continue
			}
		}

		if primaryOnly {
			if record[2] == "slave" {
				continue
			}
		}

		if replicaOnly {
			if record[2] == "master" {
				continue
			}
		}

		nodes = append(
			nodes,
			redisClusterNode{record[0], record[1], record[2], record[3], record[7]},
		)
	}

	if len(nodes) == 0 && !replicaOnly {
		return nil, errors.New("no primary nodes found in 'cluster nodes' output")
	}
	return nodes, nil
}

func (h *Client) getClusterNodes(connectedOnly, primaryOnly, replicaOnly bool) ([]redisClusterNode, error) {
	result, err := h.Client.ClusterNodes(context.Background()).Result()
	if err != nil {
		return nil, err
	}
	return parseClusterNodesResult(connectedOnly, primaryOnly, replicaOnly, result)
}

func New(conf config.RedisOpts) (*Client, error) {
	options := &redis.Options{Addr: conf.NodeAddr}

	password, err := conf.LoadPassword()
	if err != nil {
		return nil, err
	}
	options.Username = conf.Username
	options.Password = password

	tlsConfig, err := conf.LoadTLS()
	if err != nil {
		return nil, err
	}
	options.TLSConfig = tlsConfig
	return &Client{conf.NodeAddr, redis.NewClient(options)}, nil
}
