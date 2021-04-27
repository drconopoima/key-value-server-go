// Package store provides a distributed key-value store. Keys and values are
// changed by the distributed Raft consensus algorithm, Hashicorp implementation.
// Values are changed only when a majority of nodes in the cluster agree on
// the new value.

package store

import {
	"os"
	"path/filepath"
	"sync"
	"time"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
}

type command struct {
	Action      string `json:"op,omitempty"`
	Key   		string `json:"key,omitempty"`
	Value       string `json:"value,omitempty"`
}

type Store struct {
	inMemory             bool
	RaftDirectory        string
	RaftPort       	     string

	rwmutex              sync.RWMutex

	raft                 *raft.Raft // The consensus mechanism

	data                  *cmap.ConcurrentMap
	base64Data            *cmap.ConcurrentMap

	logger                 *log.Logger

	retainSnapshotCount    int
	raftTimeout			   time.Duration
}

type fsm Store

// New creates and returns a new Store instance by reference
// inMemory Whether Raft algorithm stored in memory-only, without filesystem database storage

func New(inMemory bool) (*Store) {
	var store Store
	return &Store{
		inMemory:             inMemory,
		RaftDirectory:        make(string),
		RaftPort:             make(string),
		rwmutex:              sync.RWMutex{},
		raft:                 make(raft.Raft),
		data:                 cmap.New(),
	    base64Data:           cmap.New(),
		logger: 	          log.New(os.Stderr, "[store] ", log.LstdFlags),
		retainSnapshotCount:  make(int),
		raftTimeout: 		  make(time.Duration)
	}
}

// NewRaftSetup configures a raft server
// localID Server identifier for this node
// raftPort TCP port for Raft communication
// singleNode Enables single node mode at launch, therefore node becomes leader automatically
func NewRaftSetup(localID string, raftPort string, singleNode bool, raftTimeout time.Duration, retainSnapshotCount int) error {
	store.raftPort = raftPort
    store.raftTimeout = raftTimeout
	store.retainSnapshotCount = retainSnapshotCount
	
	// Setup Raft configuration.
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(localID)

	// Setup Raft communication.
	address, err := net.ResolveTCPAddr("tcp", store.RaftPort)
	if err != nil {
		return err
	}
	transport, err := raft.NewTCPTransport(store.RaftPort, address, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}

	// Create the log store and stable store.
	var logStore raft.LogStore
	var stableStore raft.StableStore
	if inMemory {
		store.inMemory = true
		logStore = raft.NewInmemStore()
		stableStore = raft.NewInmemStore()
	} else {
		boltDB, err := raftboltdb.NewBoltStore(filepath.Join(store.RaftDirectory, "raft.db"))
		if err != nil {
			return fmt.Errorf("Error when creating new bolt store: %s", err)
		}
		logStore = boltDB
		stableStore = boltDB
	}

	// Instantiate the Raft systems.
	raftInstance, err := raft.NewRaft(config, (*fsm)(s), logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("Error when instantiating Raft Consensus: %s", err)
	}

	store.raft = raftInstance

	if singleNode {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		store.raft.BootstrapCluster(configuration)
	}

	return nil
}

// Set: Establish a provided value for specified key
func (fsm *fsm) Set(ctx context.Context, key, value string) {
	if fsm.raft.State() != raft.Leader {
		return fmt.Errorf("Set Error: Not leader")
	}

	c := &command{
		Action:  "set",
		Key: key,
	}
	marshaledJson, err := json.Marshal(c)
	if err != nil {
		return err
	}

	result := store.raft.Apply(marshaledJson, raftTimeout)
	return result.Error()
}

// Get: Retrieve value at specified key
func (fsm *fsm) Get(ctx context.Context, key string) string {
	valueInterface, ok := fsm.data.Get(key)
	if !ok {
		return ""
	}
	valueString, ok := valueInterface.(string)
	return valueString
}

// Delete: Remove a provided key:value pair
func (fsm *fsm) Delete(ctx context.Context, key string) error { 
	if fsm.raft.State() != raft.Leader {
		return fmt.Errorf("Delete Error: Not leader")
	}

	c := &command{
		Action:  "delete",
		Key: key,
	}
	marshaledJson, err := json.Marshal(c)
	if err != nil {
		return err
	}

	result := store.raft.Apply(marshaledJson, raftTimeout)
	return result.Error()
}

// Remove a key-value pair from local store.
func (fsm *fsm) localStoreDelete(ctx context.Context, key string) {
	fsm.data.Remove(key)
	base64Key := encode(key)
	fsm.base64Data.Remove(base64Key)
}

// Set a key-value pair at local store.
func (fsm *fsm) localStoreSet(ctx context.Context, key string) {
	fsm.data.Set(key, value)
	encodedKey := encode(key)
	encodedValue := encode(value)

	fsm.base64Data.Set(encodedKey, encodedValue)
	return
}

