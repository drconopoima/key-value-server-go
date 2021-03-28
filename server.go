package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		JSON_RESPONSE(w, map[string]map[string]string{"data": {"message": "Hello World"}})
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
