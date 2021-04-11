package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

var directoryName string = "testdata"

func TestJSON(test *testing.T) {
	header := http.Header{}
	headerKey := "Content-Type"
	headerValue := "application/json; charset=utf-8"
	header.Add(headerKey, headerValue)
	testCases := []struct {
		input  interface{}
		header http.Header
		output string
		status string
	}{
		{map[string]string{"message": "Hello world"}, header, `{"message":"Hello world"}`, "200 OK"},
		{map[string]map[string]string{"data": {"tables": "testing"}}, header, `{"data":{"tables":"testing"}}`, "200 OK"},
		{make(chan bool), header, `{"error":"json: unsupported type: chan bool"}`, "500 Internal Server Error"},
	}
	for _, testCase := range testCases {
		responseRecorder := httptest.NewRecorder()
		JSON(responseRecorder, testCase.input)
		response := responseRecorder.Result()
		defer response.Body.Close()
		jsonGot, err := io.ReadAll(response.Body)
		if err != nil {
			test.Fatalf("Error reading response body: %v", err)
		}
		if string(jsonGot) != testCase.output {
			test.Errorf("Output: %v, expected: %v", jsonGot, testCase.output)
		}
		if contentType := response.Header.Get(headerKey); contentType != headerValue {
			test.Errorf("Output: %v, expected: %v", contentType, headerValue)
		}
		if status, testStatus := response.Status, testCase.status; status != testStatus {
			test.Errorf("Output: %v, expected: %v", status, testStatus)
		}
	}
}

func TestGet(test *testing.T) {
	makeStorage(test)
	defer cleanupStorage(test)
	key := "key"
	value := "value"
	encodedKey := base64.URLEncoding.EncodeToString([]byte(key))
	encodedValue := base64.URLEncoding.EncodeToString([]byte(value))
	fileContents, _ := json.Marshal(map[string]string{encodedKey: encodedValue})
	dataFileName := directoryName + "/data.json"

	os.WriteFile(dataFileName, fileContents, 0644)
	loadData(dataFileName, &base64Data, &data)
	got, err := Get(context.Background(), key)
	if err != nil {
		test.Errorf("Unexpected error: %v", err)
	}
	if got != value {
		test.Errorf("Output: %v, expected: %v", got, value)
	}
}

func makeStorage(test *testing.T) {
	err := os.Mkdir(directoryName, 0755)
	if err != nil {
		test.Logf("Couldn't create directory '%v'. %v", directoryName, err)
	}
}

func cleanupStorage(test *testing.T) {
	err := os.RemoveAll(directoryName)
	if err != nil {
		test.Fatalf("Failed to delete directory '%v'. %v", directoryName, err)
	}
}
