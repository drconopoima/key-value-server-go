// Package store provides a distributed key-value store. Keys and values are
// changed by the distributed Raft consensus algorithm, Hashicorp implementation.
// Values are changed only when a majority of nodes in the cluster agree on
// the new value.

package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	cmap "github.com/orcaman/concurrent-map"
)

type command struct {
	Action string `json:"op,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}
type syncMap struct {
	dataMap map[string]string
	rwmutex sync.RWMutex
}

type Store struct {
	inMemory      bool
	RaftDirectory string
	RaftPort      string

	rwmutex sync.RWMutex

	raft *raft.Raft // The consensus mechanism

	data       cmap.ConcurrentMap
	base64Data cmap.ConcurrentMap

	logger hclog.Logger

	retainSnapshotCount int
	raftTimeout         time.Duration
}

type fsm Store

type fsmSnapshot struct {
	store map[string]string
}

// New creates and returns a new Store instance by reference
// inMemory Whether Raft algorithm stored in memory-only, without filesystem database storage
func New(inMemory bool) *Store {
	return &Store{
		inMemory:            inMemory,
		RaftDirectory:       "",
		RaftPort:            "",
		rwmutex:             sync.RWMutex{},
		data:                cmap.New(),
		base64Data:          cmap.New(),
		logger:              hclog.Default(),
		retainSnapshotCount: 2,
		raftTimeout:         10 * time.Second,
	}
}

// NewRaftSetup configures a raft server
// localID Server identifier for this node
// raftPort TCP port for Raft communication
// singleNode Enables single node mode at launch, therefore node becomes leader automatically
func NewRaftSetup(store *Store, localID string, raftPort string, singleNode bool, raftTimeout time.Duration, retainSnapshotCount int) error {
	store.RaftPort = raftPort
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
	if store.inMemory {
		store.inMemory = true
		logStore = raft.NewInmemStore()
		stableStore = raft.NewInmemStore()
	} else {
		stableDB, err := raftboltdb.NewBoltStore(filepath.Join(store.RaftDirectory, "stable", "stable.db"))
		if err != nil {
			return fmt.Errorf("Error when creating new stable bolt store: %v", err)
		}
		logDB, err := raftboltdb.NewBoltStore(filepath.Join(store.RaftDirectory, "log", "log.db"))
		if err != nil {
			return fmt.Errorf("Error when creating new log bolt store: %v", err)
		}
		logStore = logDB
		stableStore = stableDB
	}
	snapshotStore, err := raft.NewFileSnapshotStoreWithLogger(filepath.Join(store.RaftDirectory, "snaps"), 5, store.logger)
	if err != nil {
		return fmt.Errorf("Error when creating new snapshot store: %v", err)
	}
	// Instantiate the Raft systems.
	raftInstance, err := raft.NewRaft(raftConfig, (*fsm)(store), logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return fmt.Errorf("Error when instantiating Raft Consensus: %v", err)
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

// Apply function to apply a Raft log entry to the key-value store.
func (fsm *fsm) Apply(raftLog *raft.Log) interface{} {
	var command command
	if err := json.Unmarshal(raftLog.Data, &command); err != nil {
		panic(fmt.Sprintf("Error: Failed to unmarshal command %v. %v", raftLog.Data, err))
	}

	switch command.Action {
	case "set":
		fsm.localStoreSet(context.Background(), command.Key, command.Value)
		return nil
	case "delete":
		fsm.localStoreDelete(context.Background(), command.Key)
		return nil
	default:
		panic(fmt.Sprintf("Unrecognized command action: %v", command.Action))
	}
}

// Set: Establish a provided value for specified key
func (fsm *fsm) Set(ctx context.Context, key, value string) error {
	if fsm.raft.State() != raft.Leader {
		return fmt.Errorf("Set Error: Not leader")
	}

	c := &command{
		Action: "set",
		Key:    key,
	}
	marshaledJson, err := json.Marshal(c)
	if err != nil {
		return err
	}

	result := fsm.raft.Apply(marshaledJson, fsm.raftTimeout)
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
		Action: "delete",
		Key:    key,
	}
	marshaledJson, err := json.Marshal(c)
	if err != nil {
		return err
	}

	result := fsm.raft.Apply(marshaledJson, fsm.raftTimeout)
	return result.Error()
}

func encode(text string) string {
	base64Text := base64.URLEncoding.EncodeToString([]byte(text))
	return base64Text
}

func decode(text string) (string, error) {
	decodedText, err := base64.URLEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}
	return string(decodedText), nil
}

// Remove a key-value pair from local store.
func (fsm *fsm) localStoreDelete(ctx context.Context, key string) {
	fsm.data.Remove(key)
	base64Key := encode(key)
	fsm.base64Data.Remove(base64Key)
}

// Set a key-value pair at local store.
func (fsm *fsm) localStoreSet(ctx context.Context, key, value string) {
	fsm.data.Set(key, value)
	encodedKey := encode(key)
	encodedValue := encode(value)

	fsm.base64Data.Set(encodedKey, encodedValue)
	return
}

// Snapshot returns a snapshot of the key-value store.
func (fsm *fsm) Snapshot() (raft.FSMSnapshot, error) {
	snapshot_map := make(map[string]string)
	// Clone the map.
	for item := range fsm.base64Data.Iter() {
		valueInterface := item.Val
		valueString := valueInterface.(string)
		snapshot_map[item.Key] = valueString
	}
	return &fsmSnapshot{store: snapshot_map}, nil
}

// Restore Recovers a previous state of the key-value store from snapshot.
func (fsm *fsm) Restore(rc io.ReadCloser) error {
	map_restore := syncMap{
		make(map[string]string),
		sync.RWMutex{},
	}
	if err := json.NewDecoder(rc).Decode(&map_restore.dataMap); err != nil {
		return err
	}

	// Set the state from the snapshot
	waitGroupData := sync.WaitGroup{}
	waitGroupBase64Data := sync.WaitGroup{}
	map_restore.rwmutex.RLock()
	for key, value := range map_restore.dataMap {
		waitGroupData.Add(1)
		go setWorker(key, value, &fsm.base64Data, &waitGroupData)
		decodedKey, err := decode(key)
		if err != nil {
			return err
		}
		decodedValue, err := decode(value)
		if err != nil {
			return err
		}
		waitGroupBase64Data.Add(1)
		go setWorker(decodedKey, decodedValue, &fsm.base64Data, &waitGroupBase64Data)
	}
	waitGroupData.Wait()
	waitGroupBase64Data.Wait()
	map_restore.rwmutex.RUnlock()
	return nil
}

func setWorker(key, value string, data *cmap.ConcurrentMap, wg *sync.WaitGroup) error {
	defer wg.Done()
	data.Set(key, value)
	return nil
}

func (fsm *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		marshaledJson, err := json.Marshal(fsm.store)
		if err != nil {
			return err
		}

		// Write data to sink.
		if _, err := sink.Write(marshaledJson); err != nil {
			return err
		}

		// Close the sink.
		return sink.Close()
	}()

	if err != nil {
		sink.Cancel()
	}

	return err
}

func (fsm *fsmSnapshot) Release() {}
