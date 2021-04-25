// Package store provides a distributed key-value store. Keys and values are
// changed by the distributed Raft consensus algorithm, Hashicorp implementation.
// Values are changed only when a majority of nodes in the cluster agree on
// the new value.

package store

import {
	github.com/hashicorp/raft
	fsm
}

type Config struct {
	raft *raft.Raft
	fsm  *fsm
}

// NewRaftSetup configures a raft server.
// localID should be the server identifier for this node.
// raftPort is a TCP port for Raft communication
// singleNode initially enables single node mode, therefore node becomes leader
func (s *Store) Open(enableSingle bool, localID string) error {
func NewRaftSetup(localID, raftPort string, singleNode bool) (*Config, error) {
	// Setup Raft configuration.
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(localID)

	// Setup Raft communication.
	addr, err := net.ResolveTCPAddr("tcp", s.RaftBind)
	if err != nil {
		return ( *raftConfig, err )
	}
	transport, err := raft.NewTCPTransport(s.RaftBind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}
}

func (conf *Config) Set(ctx context.Context, key, value string) { }

func (conf *Config) Get(ctx context.Context, key string) string { }

func (conf *Config) Delete(ctx context.Context, key string) { }

