package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var (
	data        = map[string]string{}
	dataRWMutex = sync.RWMutex{}
)

func main() {
	// Get port from environment variables (default: 8080)
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}
	log.Printf("Starting up server on http://localhost:%s", port)
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Get("/key/{key}", func(writer_get http.ResponseWriter, request_get *http.Request) {
		key := chi.URLParam(request_get, "key")
		data, err := Get(request_get.Context(), key)
		if err != nil {
			writer_get.WriteHeader(http.StatusInternalServerError)
			JSON_RESPONSE(writer_get, map[string]string{"error": err.Error()})
			return
		}
		JSON_RESPONSE(writer_get, data)
	})
	router.Delete("/key/{key}", func(writer_delete http.ResponseWriter, request_delete *http.Request) {
		key := chi.URLParam(request_delete, "key")
		err := Delete(request_delete.Context(), key)
		if err != nil {
			writer_delete.WriteHeader(http.StatusInternalServerError)
			JSON_RESPONSE(writer_delete, map[string]string{"error": err.Error()})
			return
		}
		JSON_RESPONSE(writer_delete, map[string]string{"status": "success"})
	})
	router.Post("/key/{key}", func(writer_set http.ResponseWriter, request_set *http.Request) {
		key := chi.URLParam(request_set, "key")
		body, err := io.ReadAll(request_set.Body)
		if err != nil {
			writer_set.WriteHeader(http.StatusInternalServerError)
			JSON_RESPONSE(writer_set, map[string]string{"error": err.Error()})
			return
		}
		err = Set(request_set.Context(), key, string(body))
		if err != nil {
			writer_set.WriteHeader(http.StatusInternalServerError)
			JSON_RESPONSE(writer_set, map[string]string{"error": err.Error()})
			return
		}
		JSON_RESPONSE(writer_set, map[string]string{"status": "success"})
	})
	log.Fatal(http.ListenAndServe(":"+port, router))
}

// JSON_RESPONSE: Encode and write json data to the HTTP response
func JSON_RESPONSE(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json_bytes, err := json.Marshal(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		JSON_RESPONSE(w, map[string]string{"error": err.Error()})
	}
	w.Write(json_bytes)
}

// Get: Retrieve value at specified key
func Get(context context.Context, key string) (string, error) {
	dataRWMutex.RLock()
	defer dataRWMutex.RUnlock()

	return data[key], nil
}

// Set: Establish a provided value for specified key
func Set(context context.Context, key string, value string) error {
	dataRWMutex.Lock()
	defer dataRWMutex.Unlock()
	data[key] = value
	return nil
}

// Delete: Remove a provided key:value pair
func Delete(context context.Context, key string) error {
	dataRWMutex.Lock()
	defer dataRWMutex.Unlock()
	delete(data, key)
	return nil
}
