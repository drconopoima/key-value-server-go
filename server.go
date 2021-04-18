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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	cmap "github.com/orcaman/concurrent-map"
)

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
	storageDir := os.Getenv("STORAGE_DIR")
	dataFile := dataPath(storageDir)
	// Load data from file
	err := loadData(dataFile, &base64Data, &data)
	if err != nil {
		log.Printf("[Warning] Could not load data file %v. Error: %v", dataFile, err)
	}
	// Interval between saves to disk
	saveInterval := 60
	if saveIntervalFromEnv := os.Getenv("SAVE_INTERVAL"); saveIntervalFromEnv != "" {
		saveIntervalTry, err := strconv.Atoi(saveIntervalFromEnv)
		if err != nil {
			log.Printf("[Warning] Could not use environment value SAVE_INTERVAL %v, got error: %v. Using default value of %d", saveIntervalFromEnv, err, saveInterval)
		} else {
			saveInterval = saveIntervalTry
		}
	}
	var quitChannel chan bool
	schedule(time.Duration(saveInterval)*time.Second, saveData, &base64Data, dataFile, &quitChannel)
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/key/{key}", func(writerGet http.ResponseWriter, requestGet *http.Request) {
		key := chi.URLParam(requestGet, "key")
		dataGet := Get(requestGet.Context(), key)

		JSON(writerGet, dataGet)
	})
	router.Delete("/key/{key}", func(writerDelete http.ResponseWriter, requestDelete *http.Request) {
		key := chi.URLParam(requestDelete, "key")
		Delete(requestDelete.Context(), key)

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
		Set(requestSet.Context(), key, string(body))

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
func Get(context context.Context, key string) string {
	valueInterface, ok := data.Get(key)
	if !ok {
		return ""
	}
	valueString, ok := valueInterface.(string)
	return valueString
}

// Set: Establish a provided value for specified key
func Set(context context.Context, key, value string) {
	data.Set(key, value)
	encodedKey := encode(key)
	encodedValue := encode(value)

	base64Data.Set(encodedKey, encodedValue)
	return
}

// Delete: Remove a provided key:value pair
func Delete(context context.Context, key string) {
	data.Remove(key)
	base64Key := encode(key)
	base64Data.Remove(base64Key)
}

// dataPath: Return full path to file for data persistence storage from a directory
func dataPath(storageDir string) string {
	// Set default to '/var/tmp/'
	if storageDir == "" {
		storageDir = "/var/tmp/"
	}
	return filepath.Join(storageDir, "data.json")
}

func loadData(dataFile string, base64Data, data *cmap.ConcurrentMap) error {
	// Check if the file exists or save empty data to create.
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return saveData(base64Data, dataFile)
	}

	fileContents, err := os.ReadFile(dataFile)
	if err != nil {
		return err
	}

	var base64DataMap map[string]string = map[string]string{}
	err = json.Unmarshal(fileContents, &base64DataMap)

	if err != nil {
		return err
	}

	err = decodeWhole(base64DataMap, base64Data, data)
	if err != nil {
		log.Printf("[Warning] Could not decode base64 data from file %v. Error: %v", dataFile, err)
		return err
	}
	return nil
}

func saveData(base64Data *cmap.ConcurrentMap, dataFile string) (err error) {
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

	byteData, err := base64Data.MarshalJSON()
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
			log.Printf("[Info] Could not remove temporary file from %v. Error: %v", tmpFileName, removeErr)
			if err == nil {
				err = removeErr
			}
		}
	}()
	chmodErr := tmpFile.Chmod(fileMode)
	if chmodErr != nil {
		newFileStat, _ := tmpFile.Stat()
		newFileMode := newFileStat.Mode()
		log.Printf("[Warning] Could not preserve original file permissions of %v on %v. New file permissions: %v. Error: %v", fileMode, tmpFileName, newFileMode, chmodErr)
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

func decodeWhole(base64DataMap map[string]string, base64Data *cmap.ConcurrentMap, data *cmap.ConcurrentMap) error {
	for key, value := range base64DataMap {
		base64Data.Set(key, value)
		decodedKey, err := base64.URLEncoding.DecodeString(key)
		if err != nil {
			return err
		}
		decodedValue, err := base64.URLEncoding.DecodeString(value)
		if err != nil {
			return err
		}
		data.Set(string(decodedKey), string(decodedValue))
	}
	return nil
}

func schedule(interval time.Duration, saveData func(*cmap.ConcurrentMap, string) error, base64Data *cmap.ConcurrentMap, dataFile string, quitChannel *chan bool) {
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
