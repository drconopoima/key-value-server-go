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

var (
	data        = map[string]string{}
	dataRWMutex = sync.RWMutex{}
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
	data, _ = loadData(context.TODO(), dataFile)
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
	dataRWMutex.RLock()
	defer dataRWMutex.RUnlock()

	return data[key], nil
}

// Set: Establish a provided value for specified key
func Set(context context.Context, key, value, dataFile string) error {
	dataRWMutex.Lock()
	defer dataRWMutex.Unlock()
	data[key] = value
	err := saveData(context, data, dataFile)
	if err != nil {
		return err
	}
	return nil
}

// Delete: Remove a provided key:value pair
func Delete(context context.Context, key string, dataFile string) error {
	dataRWMutex.Lock()
	defer dataRWMutex.Unlock()
	delete(data, key)
	err := saveData(context, data, dataFile)
	if err != nil {
		return err
	}
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

func loadData(context context.Context, dataFile string) (map[string]string, error) {
	empty := map[string]string{}

	// Check if the file exists or save empty data to create.
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return empty, saveData(context, empty, dataFile)
	}

	fileContents, err := os.ReadFile(dataFile)
	if err != nil {
		return empty, err
	}

	return decode(&fileContents)
}

func saveData(context context.Context, data map[string]string, dataFile string) error {
	// Parent directory
	parentDir := filepath.Dir(dataFile)
	// Check if directory exists and create it if missing.
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		err = os.MkdirAll(parentDir, 0755)
		if err != nil {
			return err
		}
	}
	encodedData, err := encode(&data)
	if err != nil {
		return err
	}

	return os.WriteFile(dataFile, encodedData, 0644)
}

func encode(data *map[string]string) ([]byte, error) {
	encodedData := map[string]string{}
	for key, value := range *data {
		encodedKey := base64.URLEncoding.EncodeToString([]byte(key))
		encodedValue := base64.URLEncoding.EncodeToString([]byte(value))
		encodedData[encodedKey] = encodedValue
	}
	return json.Marshal(encodedData)
}

func decode(data *[]byte) (map[string]string, error) {
	var jsonData map[string]string
	if err := json.Unmarshal(*data, &jsonData); err != nil {
		return nil, err
	}
	decodedData := map[string]string{}
	for key, value := range jsonData {
		decodedKey, err := base64.URLEncoding.DecodeString(key)
		if err != nil {
			return nil, err
		}
		decodedValue, err := base64.URLEncoding.DecodeString(value)
		if err != nil {
			return nil, err
		}
		decodedData[string(decodedKey)] = string(decodedValue)
	}
	return decodedData, nil
}
