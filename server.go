package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

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
	// Interval between saves to disk
	saveInterval := 60
	if saveIntervalFromEnv := os.Getenv("SAVE_INTERVAL"); saveIntervalFromEnv != "" {
		saveIntervalTry, err := strconv.Atoi(saveIntervalFromEnv)
		if err != nil {
			log.Println("[Warning] Could not use environment SAVE_INTERVAL of", saveIntervalFromEnv, ", got error:", err.Error(), "using default value of ", saveInterval)
		} else {
			saveInterval = saveIntervalTry
		}
	}
	var quitChannel chan bool
	schedule(time.Duration(saveInterval)*time.Second, saveData, &base64Data, dataFile, &quitChannel)
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
func JSON(writer http.ResponseWriter, dataJson interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonBytes, err := json.Marshal(dataJson)
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
	data.data[key] = value
	data.mutex.Unlock()
	encodedKey := encode(key)
	encodedValue := encode(value)

	base64Data.mutex.Lock()
	base64Data.data[encodedKey] = encodedValue
	base64Data.mutex.Unlock()
	return nil
}

// Delete: Remove a provided key:value pair
func Delete(context context.Context, key string, dataFile string) error {
	data.mutex.Lock()
	delete(data.data, key)
	data.mutex.Unlock()
	base64Key := encode(key)
	base64Data.mutex.Lock()
	delete(base64Data.data, base64Key)
	base64Data.mutex.Unlock()
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
	if err := json.Unmarshal(fileContents, &base64Data.data); err != nil {
		return err
	}
	return nil
}

func saveData(base64Data *dataVessel, dataFile string) (err error) {
	// Parent directory
	parentDir := filepath.Dir(dataFile)
	dataFileName := filepath.Base(dataFile)
	// Check if directory exists and create it if missing.
	if _, err = os.Stat(parentDir); os.IsNotExist(err) {
		err = os.MkdirAll(parentDir, 0755)
		if err != nil {
			return
		}
	}
	// Preserve existing permissions, default 0644
	var fileMode fs.FileMode = 0644
	fileInfo, err := os.Stat(dataFile)
	if err == nil {
		fileMode = fileInfo.Mode().Perm()
	}
	// Write into temporary file
	tmpDataFilePattern := dataFileName + "*.tmp"
	tmpFile, err := os.CreateTemp(parentDir, tmpDataFilePattern)
	if err != nil {
		return
	}
	tmpFileName := tmpFile.Name()
	base64Data.mutex.RLock()
	byteData, err := json.Marshal(base64Data.data)
	base64Data.mutex.RUnlock()
	if err != nil {
		return
	}
	defer func() {
		// Catch error in file close
		// Reference: https://www.joeshaw.org/dont-defer-close-on-writable-files/
		closeErr := tmpFile.Close()
		if err == nil {
			err = closeErr
		}
		// Ensure the Temporary file is cleared
		// Most of the time throws warning because it was successfully renamed already
		// Left for clean-up if the rename step failed
		removeErr := os.Remove(tmpFileName)
		if removeErr != nil {
			log.Println("[Info] Could not remove temporary file", tmpFileName, ". Error:", removeErr.Error())
			if err == nil {
				err = removeErr
			}
		}
	}()
	chmodErr := tmpFile.Chmod(fileMode)
	if chmodErr != nil {
		newFileStat, _ := tmpFile.Stat()
		newFileMode := newFileStat.Mode()
		log.Println("[Warning] Could not preserve original file permissions on", tmpFileName, "of ", fileMode, ". New file permissions: ", newFileMode, ". Error: ", chmodErr.Error())
	}
	_, err = tmpFile.Write(byteData)
	if err != nil {
		return
	}
	err = tmpFile.Sync()
	if err != nil {
		return
	}
	// Rename temporary file for atomicity
	err = os.Rename(tmpFileName, dataFile)
	return
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

func schedule(interval time.Duration, saveData func(*dataVessel, string) error, base64Data *dataVessel, dataFile string, quitChannel *chan bool) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			select {
			case <-*quitChannel:
				return
			default:
				saveData(base64Data, dataFile)
			}
		}
	}()
}
