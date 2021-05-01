package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/drconopoima/key-value-server-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	cmap "github.com/orcaman/concurrent-map"
)

type syncMap struct {
	dataMap map[string]string
	rwmutex sync.RWMutex
}

var (
	data       = cmap.New()
	base64Data = cmap.New()
)

func main() {
	// Get port from PORT environment variables (default: 8080)
	port := "8080"
	if portFromEnv := os.Getenv("PORT"); portFromEnv != "" {
		port = portFromEnv
	}
	log.Printf("Starting up server on http://localhost:%v", port)
	// Get persistence file from STORAGE_DIR environment variable
	// (default: /var/tmp defined within func dataPath)
	storageDir := os.Getenv("STORAGE_DIR")

	raftAddress := "localhost"
	if raftAddressFromEnv := os.Getenv("RAFT_ADDRESS"); raftAddressFromEnv != "" {
		raftAddress = raftAddressFromEnv
	}

	raftPort := "9080"
	if raftPortFromEnv := os.Getenv("RAFT_PORT"); raftPortFromEnv != "" {
		raftPort = raftPortFromEnv
	}

	singleNode := true
	if singleNodeString := os.Getenv("SINGLE_NODE"); singleNodeString != "" {
		singleNodeString = strings.ToLower(singleNodeString)
		if singleNodeString == "false" {
			singleNode = false
		}
		if singleNodeString == "true" {
			singleNode = true
		}
	}

	localIdFromEnv := os.Getenv("RAFT_ID")
	if localIdFromEnv == "" {
		localIdFromEnv = uuid.New().URN()
	}
	localID := localIdFromEnv

	raftInMemory := true
	if raftInMemoryString := os.Getenv("RAFT_INMEMORY"); raftInMemoryString != "" {
		raftInMemoryString = strings.ToLower(raftInMemoryString)
		if raftInMemoryString == "false" {
			raftInMemory = false
		}
		if raftInMemoryString == "true" {
			raftInMemory = true
		}
	}

	store := store.New()
	store.RaftAddress = raftAddress
	store.RaftPort = raftPort
	store.RaftDirectory = storageDir

	err := store.StartRaft(localID, singleNode, raftInMemory)
	if err != nil {
		log.Fatalf("Error while setting up Raft: %v", err)
	}
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/key/{key}", func(writerGet http.ResponseWriter, requestGet *http.Request) {
		key := chi.URLParam(requestGet, "key")
		dataGet := store.Get(key)

		JSON(writerGet, dataGet)
	})
	router.Delete("/key/{key}", func(writerDelete http.ResponseWriter, requestDelete *http.Request) {
		key := chi.URLParam(requestDelete, "key")
		store.Delete(key)

		JSON(writerDelete, map[string]string{"status": "success"})
	})
	router.Post("/key/{key}", func(writerSet http.ResponseWriter, requestSet *http.Request) {
		key := chi.URLParam(requestSet, "key")
		body, err := io.ReadAll(requestSet.Body)
		if err != nil {
			writerSet.WriteHeader(http.StatusInternalServerError)
			JSON(writerSet, map[string]string{"error": err.Error()})
			return
		}
		store.Set(key, string(body))

		JSON(writerSet, map[string]string{"status": "success"})
	})
	// Start HTTP server on all interfaces
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// JSON: Encode and write json data to the HTTP response
func JSON(writer http.ResponseWriter, dataJson interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonBytes, err := json.Marshal(dataJson)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		JSON(writer, map[string]string{"error": err.Error()})
	}
	writer.Write(jsonBytes)
}
