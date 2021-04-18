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
	test.Parallel()
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
	test.Parallel()
	keyValueStore := map[string]string{
		"key1": "value1",
		"key3": "value3",
	}
	base64EncodedStore := map[string]string{}
	for key, value := range keyValueStore {
		encodedKey := base64.URLEncoding.EncodeToString([]byte(key))
		encodedValue := base64.URLEncoding.EncodeToString([]byte(value))
		base64EncodedStore[encodedKey] = encodedValue
	}
	err := callLoadData(test, &base64EncodedStore)
	if err != nil {
		test.Fatalf("Couldn't load test data from file %v. Error: %v", directoryName+"/data.json", err)
	}
	testCases := []struct {
		key   string
		value string
		err   error
	}{
		{"key1", keyValueStore["key1"], nil},
		{"key2", keyValueStore["key2"], nil},
		{"key3", keyValueStore["key3"], nil},
	}
	for _, testCase := range testCases {
		valueGot := Get(context.Background(), testCase.key)
		if valueGot != testCase.value {
			test.Errorf("Output: %v, expected: %v", valueGot, testCase.value)
		}
	}
}

func makeStorage(testbench testing.TB) {
	err := os.Mkdir(directoryName, 0755)
	if err != nil {
		testbench.Logf("Couldn't create directory '%v'. %v", directoryName, err)
	}
}

func cleanupStorage(testbench testing.TB, filePath string) {
	err := os.RemoveAll(filePath)
	if err != nil {
		testbench.Fatalf("Failed to delete path '%v'. %v", filePath, err)
	}
}

func callLoadData(testbench testing.TB, base64EncodedStore *map[string]string) error {
	makeStorage(testbench)
	dataFileName := directoryName + "/data.json"
	defer cleanupStorage(testbench, dataFileName)
	fileContents, err := json.Marshal(base64EncodedStore)
	if err != nil {
		return err
	}
	err = os.WriteFile(dataFileName, fileContents, 0644)
	if err != nil {
		return err
	}
	return loadData(dataFileName, &base64Data, &data)
}

func BenchmarkGet(bench *testing.B) {
	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		Get(context.Background(), "key1")
	}
}
