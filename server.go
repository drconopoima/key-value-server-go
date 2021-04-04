package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type dataVessel struct {
	data  map[string]string
	mutex sync.RWMutex
}

var (
	data = dataVessel{
		data:  map[string]string{},
		mutex: sync.RWMutex{},
	}
	base64Data = dataVessel{
		data:  map[string]string{},
		mutex: sync.RWMutex{},
	}
)

func main() {
	// Get port from PORT environment variables (default: 8080)
	port := "8080"
	if portFromEnv := os.Getenv("PORT"); portFromEnv != "" {
		port = portFromEnv
	}
	log.Printf("Starting up server on http://localhost:%s", port)
	// Get persistence file from STORAGE_DIR environment variable
	storageDir := os.Getenv("STORAGE_DIR")
	dataFile := dataPath(storageDir)
	// Load data from file
	err := loadData(dataFile, &base64Data)
	if err != nil {
		log.Println("[Warning] Could not load data file", dataFile, err.Error())
	}
	err = decodeWhole(&base64Data, &data)
	if err != nil {
		log.Println("[Warning] Could not decode base64 data from file", err.Error())
	}
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/key/{key}", func(writerGet http.ResponseWriter, requestGet *http.Request) {
		key := chi.URLParam(requestGet, "key")
		dataGet, err := Get(requestGet.Context(), key, dataFile)
		if err != nil {
			writerGet.WriteHeader(http.StatusInternalServerError)
			JSON(writerGet, map[string]string{"error": err.Error()})
			return
		}
		JSON(writerGet, dataGet)
	})
	router.Delete("/key/{key}", func(writerDelete http.ResponseWriter, requestDelete *http.Request) {
		key := chi.URLParam(requestDelete, "key")
		err := Delete(requestDelete.Context(), key, dataFile)
		if err != nil {
			writerDelete.WriteHeader(http.StatusInternalServerError)
			JSON(writerDelete, map[string]string{"error": err.Error()})
			return
		}
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
		err = Set(requestSet.Context(), key, string(body), dataFile)
		if err != nil {
			writerSet.WriteHeader(http.StatusInternalServerError)
			JSON(writerSet, map[string]string{"error": err.Error()})
			return
		}
		JSON(writerSet, map[string]string{"status": "success"})
	})
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// JSON: Encode and write json data to the HTTP response
func JSON(writer http.ResponseWriter, data interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		JSON(writer, map[string]string{"error": err.Error()})
	}
	writer.Write(jsonBytes)
}

// Get: Retrieve value at specified key
func Get(context context.Context, key, dataFile string) (string, error) {
	data.mutex.RLock()
	defer data.mutex.RUnlock()

	return data.data[key], nil
}

// Set: Establish a provided value for specified key
func Set(context context.Context, key, value, dataFile string) error {
	data.mutex.Lock()
	defer data.mutex.Unlock()
	data.data[key] = value
	encodedKey := encode(key)
	encodedValue := encode(value)
	base64Data.data[encodedKey] = encodedValue

	return nil
}

// Delete: Remove a provided key:value pair
func Delete(context context.Context, key string, dataFile string) error {
	data.mutex.Lock()
	defer data.mutex.Unlock()
	delete(data.data, key)
	base64Key := encode(key)
	delete(base64Data.data, base64Key)
	return nil
}

// dataPath: Return full path to file for data persistence storage from a directory
func dataPath(storageDir string) string {
	// Set default to '/var/tmp/'
	if storageDir == "" {
		storageDir = "/var/tmp/"
	}
	return filepath.Join(storageDir, "data.json")
}

func loadData(dataFile string, base64Data *dataVessel) error {
	// Check if the file exists or save empty data to create.
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return saveData(base64Data, dataFile)
	}

	fileContents, err := os.ReadFile(dataFile)
	if err != nil {
		return err
	}

	base64Data.mutex.Lock()
	defer base64Data.mutex.Unlock()
	if err := json.Unmarshal(fileContents, base64Data); err != nil {
		return err
	}
	return nil
}

func saveData(base64Data *dataVessel, dataFile string) error {
	// Parent directory
	parentDir := filepath.Dir(dataFile)
	// Check if directory exists and create it if missing.
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		err = os.MkdirAll(parentDir, 0755)
		if err != nil {
			return err
		}
	}
	base64Data.mutex.RLock()
	defer base64Data.mutex.RUnlock()
	byteData, err := json.Marshal(base64Data)
	if err != nil {
		return err
	}
	return os.WriteFile(dataFile, byteData, 0644)
}

func encode(text string) string {
	base64Text := base64.URLEncoding.EncodeToString([]byte(text))
	return base64Text
}

func decodeWhole(base64Data *dataVessel, data *dataVessel) error {
	base64Data.mutex.RLock()
	defer base64Data.mutex.RUnlock()
	data.mutex.Lock()
	defer data.mutex.Unlock()
	for key, value := range base64Data.data {
		decodedKey, err := base64.URLEncoding.DecodeString(key)
		if err != nil {
			return err
		}
		decodedValue, err := base64.URLEncoding.DecodeString(value)
		if err != nil {
			return err
		}
		(data.data)[string(decodedKey)] = string(decodedValue)
	}
	return nil
}
