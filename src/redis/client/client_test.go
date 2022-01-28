package client

import (
	"reflect"
	"testing"
)

func Test_parseClusterNodesResult(t *testing.T) {
	type args struct {
		connectedOnly bool
		primaryOnly   bool
		replicaOnly   bool
		result        string
	}
	tests := []struct {
		args    args
		want    []redisClusterNode
		wantErr bool
	}{
		{
			args{
				connectedOnly: true,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "237c7223aa3bfae4d0b9ac2c7e1990c46b33ee73 127.0.0.1:31264@41264 myself,slave 59e29b0b4fc1c6f5f2c2698ffdda28cd00f77510 0 1637115996000 7 connected",
			},
			[]redisClusterNode{
				{
					"237c7223aa3bfae4d0b9ac2c7e1990c46b33ee73",
					"127.0.0.1:31264@41264",
					"slave",
					"59e29b0b4fc1c6f5f2c2698ffdda28cd00f77510",
					"connected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: false,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "58ee2b950353f1ba209a7cbe016d424550265312 127.0.0.1:27356@37356 master,fail - 1637115881821 0 0 disconnected",
			},
			[]redisClusterNode{
				{
					"58ee2b950353f1ba209a7cbe016d424550265312",
					"127.0.0.1:27356@37356",
					"master",
					"-",
					"disconnected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: true,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "fb41fa1d1f85a33be723fa2e553cd78bd6846017 127.0.0.1:28216@38216 master - 0 1637116000822 0 connected",
			},
			[]redisClusterNode{
				{
					"fb41fa1d1f85a33be723fa2e553cd78bd6846017",
					"127.0.0.1:28216@38216",
					"master",
					"-",
					"connected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: true,
				primaryOnly:   true,
				replicaOnly:   false,
				result:        "59e29b0b4fc1c6f5f2c2698ffdda28cd00f77510 127.0.0.1:28999@38999 master - 0 1637116000000 7 connected 0-5460",
			},
			[]redisClusterNode{
				{
					"59e29b0b4fc1c6f5f2c2698ffdda28cd00f77510",
					"127.0.0.1:28999@38999",
					"master",
					"-",
					"connected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: true,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "57d589837c2f39e0959ee49c5dd3f0ed09da0b20 127.0.0.1:20449@30449 master,fail - 1637115880786 0 0 disconnected",
			},
			nil,
			true,
		},
		{
			args{
				connectedOnly: false,
				primaryOnly:   false,
				replicaOnly:   true,
				result:        "a9b3c447fcf74ef7e49756fa35b13dbf03a3fd16 127.0.0.1:26437@36437 slave,fail - 1637115881821 0 0 disconnected",
			},
			[]redisClusterNode{
				{
					"a9b3c447fcf74ef7e49756fa35b13dbf03a3fd16",
					"127.0.0.1:26437@36437",
					"slave",
					"-",
					"disconnected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: true,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "a7b72b6332f6890c195e4c4504b538480b964a0c 127.0.0.1:28896@38896 slave 2688d5779f45312a39ab2ce7aaa777839097c993 0 1637116003936 8 connected",
			},
			[]redisClusterNode{
				{
					"a7b72b6332f6890c195e4c4504b538480b964a0c",
					"127.0.0.1:28896@38896",
					"slave",
					"2688d5779f45312a39ab2ce7aaa777839097c993",
					"connected",
				},
			},
			false,
		},
		{
			args{
				connectedOnly: false,
				primaryOnly:   false,
				replicaOnly:   false,
				result:        "59ec374a0f482e2959d669065e6ac137d94a5df1 127.0.0.1:28395@38395 master,fail - 1637115881821 0 0 disconnected",
			},
			[]redisClusterNode{
				{
					"59ec374a0f482e2959d669065e6ac137d94a5df1",
					"127.0.0.1:28395@38395",
					"master",
					"-",
					"disconnected",
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := parseClusterNodesResult(tt.args.connectedOnly, tt.args.primaryOnly, tt.args.replicaOnly, tt.args.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseClusterNodesResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseClusterNodesResult() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_unmarshalClusterInfo(t *testing.T) {
	type args struct {
		result string
	}
	tests := []struct {
		args    args
		want    *clusterInfo
		wantErr bool
	}{
		{
			args{"cluster_state:ok\r\ncluster_slots_assigned:16384\r\ncluster_slots_ok:16384\r\ncluster_slots_pfail:0\r\ncluster_slots_fail:0\r\ncluster_known_nodes:13\r\ncluster_size:3\r\ncluster_current_epoch:10\r\ncluster_my_epoch:7\r\ncluster_stats_messages_ping_sent:88\r\ncluster_stats_messages_pong_sent:63\r\ncluster_stats_messages_meet_sent:1\r\ncluster_stats_messages_sent:152\r\ncluster_stats_messages_ping_received:63\r\ncluster_stats_messages_pong_received:82\r\ncluster_stats_messages_received:145\r\n"},
			&clusterInfo{
				State:                 "ok",
				SlotsAssigned:         16384,
				SlotsOk:               16384,
				SlotsPfail:            0,
				SlotsFail:             0,
				KnownNodes:            13,
				Size:                  3,
				CurrentEpoch:          10,
				MyEpoch:               7,
				StatsMessagesSent:     152,
				StatsMessagesReceived: 145,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := unmarshalClusterInfo(tt.args.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalClusterInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unmarshalClusterInfo() = %+v, want %v", got, tt.want)
			}
		})
	}
}
