package main

// key-value-server-go is a simple, concurrent key-value HTTP server with persistent storage.
// It provides RESTful endpoints for storing, retrieving, and deleting key-value pairs.
//
// Features:
//   - Concurrent-safe operations using concurrent maps
//   - Automatic data persistence to disk at configurable intervals
//   - Base64 encoding for safe storage of arbitrary binary data
//   - Configurable via environment variables
//
// Environment Variables:
//   - PORT: HTTP server port (default: 8080)
//   - ADDRESS: HTTP server address/hostname (default: empty, binds to all interfaces)
//   - STORAGE_DIR: Directory for persistent data storage (default: /var/tmp/)
//   - SAVE_INTERVAL: Interval in seconds between automatic saves (default: 60)
//
// API Endpoints:
//   - GET  /key/{key}    - Retrieve the value for a given key
//   - POST /key/{key}    - Set the value for a given key (body is the value)
//   - DELETE /key/{key} - Delete a key-value pair
//
// Responses:
//   - GET returns the value as JSON if found, or 404 if key doesn't exist
//   - POST returns {"status": "success"} on success, or 500 on error
//   - DELETE returns {"status": "success"} on success
//
// Persistence:
//   - Data is automatically loaded from STORAGE_DIR/data.json on startup
//   - Data is automatically saved to disk every SAVE_INTERVAL seconds
//   - Data is stored in base64-encoded format for compatibility
//   - File writes are atomic (uses temp file + rename pattern)

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
	cmap "github.com/drconopoima/concurrent-map/v2"
)

// syncMap is a thread-safe map wrapper for JSON marshaling/unmarshaling.
// It is used as an intermediate structure when loading/saving data to disk.
type syncMap struct {
	dataMap map[string]string
	rwmutex sync.RWMutex
}

var (
	// data is the primary in-memory key-value store.
	// Uses concurrent-map for thread-safe concurrent access.
	data = cmap.New[string]()

	// base64Data is the base64-encoded copy of data, used for persistent storage.
	// Both data and base64Data are kept in sync to ensure data can be
	// correctly saved and loaded. Keys and values are base64-encoded to handle
	// arbitrary byte data safely in JSON format.
	base64Data = cmap.New[string]()
)

// main initializes the HTTP server, loads persistent data, and starts the service.
//
// Behavior:
//   - Reads configuration from environment variables (PORT, ADDRESS, STORAGE_DIR, SAVE_INTERVAL)
//   - Loads existing data from the persistence file if it exists
//   - Creates a new empty data file if STORAGE_DIR exists but data.json doesn't
//   - Logs a warning (but continues) if data file cannot be loaded
//   - Starts a background goroutine that saves data to disk at the configured interval
//   - Sets up HTTP routes for GET, POST, and DELETE operations on /key/{key}
//   - Uses chi router with built-in request logging middleware
//   - Binds to ADDRESS:PORT or :PORT (all interfaces) if ADDRESS is not set
func main() {
	// Get port from PORT environment variables (default: 8080)
	port := "8080"
	if portFromEnv := os.Getenv("PORT"); portFromEnv != "" {
		port = portFromEnv
	}
	address := ""
	if addressFromEnv := os.Getenv("ADDRESS"); addressFromEnv != "" {
		address = addressFromEnv
	}
	if address != "" {
		log.Printf("Starting up server on http://%v:%v", address, port)
	} else {
		log.Printf("Starting up server on http://0.0.0.0:%v", port)
	}
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
	// GET /key/{key} - Retrieve a value by key.
	// Returns the value as JSON if found, or 404 Not Found if the key doesn't exist.
	// The value is returned as-is (as a string).
	router.Get("/key/{key}", func(writerGet http.ResponseWriter, requestGet *http.Request) {
		key := chi.URLParam(requestGet, "key")
		dataGet := Get(requestGet.Context(), key)
		if dataGet != nil {
			JSON(writerGet, dataGet)
		} else {
			writerGet.WriteHeader(http.StatusNotFound)
		}

	})
	// DELETE /key/{key} - Remove a key-value pair.
	// Returns {"status": "success"} as JSON on success.
	// Does not return an error if the key doesn't exist (idempotent operation).
	// The delete is synchronous and immediate in memory; persistence to disk
	// happens asynchronously via the scheduled save.
	router.Delete("/key/{key}", func(writerDelete http.ResponseWriter, requestDelete *http.Request) {
		key := chi.URLParam(requestDelete, "key")
		Delete(requestDelete.Context(), key)

		JSON(writerDelete, map[string]string{"status": "success"})
	})
	// POST /key/{key} - Set or update a value for a key.
	// The request body is used as the value (read entirely as a string).
	// Returns {"status": "success"} as JSON on success.
	// Returns 500 Internal Server Error if the request body cannot be read.
	// The value is stored immediately in memory and will be persisted to disk
	// at the next scheduled save interval.
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
	if address != "" {
		log.Fatal(http.ListenAndServe(address+":"+port, router))
	} else {
		log.Fatal(http.ListenAndServe(":"+port, router))
	}
}

// JSON encodes the given data as JSON and writes it to the HTTP response.
// It sets the Content-Type header to "application/json; charset=utf-8".
// If JSON marshaling fails, it writes a 500 Internal Server Error response
// with the error message as JSON.
//
// Note: This function is recursive - if marshaling fails, it calls itself
// to write the error. This works because the error case uses a simple
// map[string]string which is always marshalable.
func JSON(writer http.ResponseWriter, dataJson interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonBytes, err := json.Marshal(dataJson)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		JSON(writer, map[string]string{"error": err.Error()})
	}
	writer.Write(jsonBytes)
}

// Get retrieves the value associated with the given key from the in-memory store.
//
// Parameters:
//   - context: The request context (currently unused but kept for API consistency)
//   - key: The key to look up
//
// Returns:
//   - The value as an interface{} (actually string) if the key exists
//   - nil if the key does not exist
//
// This is a thread-safe operation using the concurrent map.
func Get(context context.Context, key string) interface{} {
	valueString, ok := data.Get(key)
	if !ok {
		return nil
	}
	return valueString
}

// Set stores a key-value pair in the in-memory store.
//
// Parameters:
//   - context: The request context (currently unused but kept for API consistency)
//   - key: The key to set
//   - value: The value to associate with the key
//
// Behavior:
//   - Stores the key-value pair in the primary `data` map
//   - Also stores a base64-encoded version in `base64Data` map for persistence
//   - Both maps are updated atomically to keep them in sync
//   - If the key already exists, its value is overwritten
//
// This is a thread-safe operation using concurrent maps.
func Set(context context.Context, key, value string) {
	data.Set(key, value)
	encodedKey := encode(key)
	encodedValue := encode(value)

	base64Data.Set(encodedKey, encodedValue)
	return
}

// Delete removes a key-value pair from the in-memory store.
//
// Parameters:
//   - context: The request context (currently unused but kept for API consistency)
//   - key: The key to delete
//
// Behavior:
//   - Removes the key from both the primary `data` map and the `base64Data` map
//   - The base64-encoded version of the key is used to remove from base64Data
//   - If the key doesn't exist, this is a no-op (idempotent)
//
// This is a thread-safe operation using concurrent maps.
func Delete(context context.Context, key string) {
	data.Remove(key)
	base64Key := encode(key)
	base64Data.Remove(base64Key)
}

// dataPath returns the full path to the data persistence file.
//
// Parameters:
//   - storageDir: The directory where data should be stored
//
// Returns:
//   - The full path as storageDir + "/data.json"
//   - If storageDir is empty, uses "/var/tmp/" as the default directory
//
// Note: The default directory /var/tmp/ is a common location for temporary
// files that persist across reboots on many Unix-like systems.
func dataPath(storageDir string) string {
	// Set default to '/var/tmp/'
	if storageDir == "" {
		storageDir = "/var/tmp/"
	}
	return filepath.Join(storageDir, "data.json")
}

// loadData loads persistent data from the data file into memory.
//
// Parameters:
//   - dataFile: The path to the JSON data file
//   - base64Data: Pointer to the base64-encoded data map (will be populated)
//   - data: Pointer to the primary data map (will be populated)
//
// Returns:
//   - error if the file cannot be read or parsed
//
// Behavior:
//   - If the data file doesn't exist, creates an empty file and returns nil
//   - Reads and parses the JSON file into a temporary syncMap
//   - Uses decodeWhole to decode base64 values and populate both maps
//   - If decoding fails, logs a warning but still returns the error
//   - Thread-safe: uses RWMutex locks on the temporary map during unmarshaling
//
// Note: The file format is a JSON object with base64-encoded keys and values.
// This encoding allows arbitrary byte data (including special characters)
// to be safely stored in JSON format.
func loadData(dataFile string, base64Data, data *cmap.ConcurrentMap[string, string]) error {
	// Check if the file exists or save empty data to create.
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return saveData(base64Data, dataFile)
	}

	fileContents, err := os.ReadFile(dataFile)
	if err != nil {
		return err
	}

	var base64DataMap syncMap = syncMap{map[string]string{}, sync.RWMutex{}}
	base64DataMap.rwmutex.Lock()
	err = json.Unmarshal(fileContents, &base64DataMap.dataMap)
	base64DataMap.rwmutex.Unlock()
	if err != nil {
		return err
	}

	err = decodeWhole(&base64DataMap, base64Data, data)
	if err != nil {
		log.Printf("[Warning] Could not decode base64 data from file %v. Error: %v", dataFile, err)
		return err
	}
	return nil
}

// saveData persists the current state of base64Data to disk.
//
// Parameters:
//   - base64Data: Pointer to the base64-encoded data map to save
//   - dataFile: The destination file path
//
// Returns:
//   - error if any step in the save process fails
//
// Behavior:
//   - Creates parent directories if they don't exist (with 0755 permissions)
//   - Preserves existing file permissions if the file already exists
//   - Uses atomic write pattern: write to temp file, then rename to target
//   - Writes data as JSON with base64-encoded keys and values
//   - Ensures data is synced to disk (fsync) before rename
//   - Cleans up temp file after rename (best effort)
//
// Atomicity: The rename operation is atomic on most filesystems,
// ensuring that readers either see the old data or the new data, never
// a partially written file.
//
// Note: If file permissions cannot be preserved, a warning is logged
// but the save continues.
func saveData(base64Data *cmap.ConcurrentMap[string, string], dataFile string) (err error) {
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

// encode converts a string to its base64 URL-encoded representation.
//
// Parameters:
//   - text: The string to encode
//
// Returns:
//   - The base64 URL-encoded string
//
// Note: Uses URLEncoding (not StdEncoding) which uses - and _ instead of +
// and /, and omits padding. This is safe for use in URLs and JSON.
func encode(text string) string {
	base64Text := base64.URLEncoding.EncodeToString([]byte(text))
	return base64Text
}

// decodeWhole decodes all key-value pairs from a syncMap into both data maps.
//
// Parameters:
//   - base64DataMap: Pointer to the source syncMap with base64-encoded data
//   - base64Data: Pointer to the target base64-encoded concurrent map
//   - data: Pointer to the target decoded concurrent map
//
// Returns:
//   - error (always nil, errors from individual workers are ignored)
//
// Behavior:
//   - Spawns a goroutine for each key-value pair to decode concurrently
//   - Each goroutine decodes its pair and adds to both maps
//   - Uses WaitGroup to wait for all goroutines to complete
//   - Holds a read lock on the source map during iteration
//   - Errors from individual decode operations are returned but not collected
//
// Note: This function uses concurrent decoding for better performance
// when loading large datasets.
func decodeWhole(base64DataMap *syncMap, base64Data *cmap.ConcurrentMap[string, string], data *cmap.ConcurrentMap[string, string]) error {
	waitGroup := sync.WaitGroup{}

	base64DataMap.rwmutex.RLock()
	for key, value := range base64DataMap.dataMap {
		waitGroup.Add(1)
		go decodeWorker(key, value, &waitGroup)
	}
	waitGroup.Wait()
	base64DataMap.rwmutex.RUnlock()
	return nil
}

// decodeWorker decodes a single base64-encoded key-value pair and stores
// it in both the base64 and decoded maps.
//
// Parameters:
//   - key: The base64-encoded key
//   - value: The base64-encoded value
//   - wg: WaitGroup to signal completion
//
// Returns:
//   - error if base64 decoding fails for either key or value
//
// Behavior:
//   - Stores the original base64-encoded key-value in base64Data map
//   - Decodes both key and value from base64
//   - Stores the decoded key-value in the primary data map
//   - Errors from decoding are returned but not handled (caller ignores them)
//
// Note: This is designed to be called concurrently from decodeWhole.
// Each worker is independent and thread-safe.
func decodeWorker(key, value string, wg *sync.WaitGroup) error {
	defer wg.Done()
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
	return nil
}

// schedule starts a background goroutine that periodically saves data to disk.
//
// Parameters:
//   - interval: The time duration between save operations
//   - saveData: The function to call for saving (signature: func(*cmap.ConcurrentMap[string, string], string) error)
//   - base64Data: Pointer to the base64-encoded data map to save
//   - dataFile: The destination file path
//   - quitChannel: Pointer to a channel that can be used to stop the scheduler
//
// Behavior:
//   - Creates a ticker that fires at the specified interval
//   - On each tick, calls saveData(base64Data, dataFile) in the background
//   - Uses a select statement to check for quit signal before each save
//   - If quitChannel receives a value, the goroutine exits
//   - Errors from saveData are ignored (no error handling)
//
// Note: The quitChannel is currently never populated (var quitChannel chan bool
// creates a nil channel), so the scheduler cannot be stopped gracefully.
// The goroutine will run indefinitely until the program exits.
//
// Concurrency: The save operation runs in a separate goroutine, so it
// does not block the main thread. Multiple saves could potentially
// overlap if saveData takes longer than the interval.
func schedule(interval time.Duration, saveData func(*cmap.ConcurrentMap[string, string], string) error, base64Data *cmap.ConcurrentMap[string, string], dataFile string, quitChannel *chan bool) {
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
