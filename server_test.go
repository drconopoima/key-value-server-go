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

	cmap "github.com/orcaman/concurrent-map"
)

var directoryName string = "testdata"

func TestJSON(test *testing.T) {
	test.Parallel()
	header := http.Header{}
	headerKey := "Content-Type"
	headerValue := "application/json; charset=utf-8"
	header.Add(headerKey, headerValue)
	firstHashMap := cmap.New()
	firstHashMap.Set("message", "Hello world")
	secondHashMap := cmap.New()
	secondHashMap.Set("data", map[string]string{"tables": "testing"})
	testCases := []struct {
		input  interface{}
		header http.Header
		output string
		status string
	}{
		{firstHashMap, header, `{"message":"Hello world"}`, "200 OK"},
		{secondHashMap, header, `{"data":{"tables":"testing"}}`, "200 OK"},
		{make(chan bool), header, `{"error":"json: unsupported type: chan bool"}`, "500 Internal Server Error"},
	}
	for _, testCase := range testCases {
		responseRecorder := httptest.NewRecorder()
		var toMap interface{}
		concurrentMap, ok := testCase.input.(cmap.ConcurrentMap)
		if ok {
			for key := range concurrentMap.IterBuffered() {
				value := key.Val
				toMap = map[string]interface{}{key.Key: value}
			}
		} else {
			toMap = testCase.input
		}

		JSON(responseRecorder, toMap)
		response := responseRecorder.Result()
		defer response.Body.Close()
		gotBytes, err := io.ReadAll(response.Body)
		if err != nil {
			test.Fatalf("Error reading response body: %v", err)
		}
		jsonGot := string(gotBytes)
		if jsonGot != testCase.output {
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
	keyValueStore := cmap.New()
	keyValueStore.Set("key1", "value1")
	keyValueStore.Set("key3", "value3")
	base64EncodedStore := cmap.New()
	for keyValueStruct := range keyValueStore.IterBuffered() {
		encodedKey := base64.URLEncoding.EncodeToString([]byte(keyValueStruct.Key))
		encodedValue := base64.URLEncoding.EncodeToString([]byte(keyValueStruct.Val.(string)))
		base64EncodedStore.Set(encodedKey, encodedValue)
	}
	err := callLoadData(test, &base64EncodedStore)
	if err != nil {
		test.Fatalf("Couldn't load test data from file %v. Error: %v", directoryName+"/data.json", err)
	}
	value1Interface, _ := keyValueStore.Get("key1")
	value1, ok := value1Interface.(string)
	if !ok {
		value1 = ""
	}
	value2Interface, _ := keyValueStore.Get("key2")
	value2, ok := value2Interface.(string)
	if !ok {
		value2 = ""
	}
	value3Interface, _ := keyValueStore.Get("key3")
	value3, ok := value3Interface.(string)
	if !ok {
		value3 = ""
	}
	testCases := []struct {
		key   string
		value string
		err   error
	}{
		{"key1", value1, nil},
		{"key2", value2, nil},
		{"key3", value3, nil},
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

func cleanupStorage(testbench testing.TB, fileName string) {
	err := os.RemoveAll(fileName)
	if err != nil {
		testbench.Fatalf("Failed to delete directory '%v'. %v", fileName, err)
	}
}

func callLoadData(testbench testing.TB, base64EncodedStore *cmap.ConcurrentMap) error {
	makeStorage(testbench)
	// defer cleanupStorage(testbench, directoryName+"/data.json")
	var base64EncodedStoreMap map[string]string = map[string]string{}
	for keyValueStruct := range base64EncodedStore.Iter() {
		base64EncodedStoreMap[keyValueStruct.Key] = keyValueStruct.Val.(string)
	}
	fileContents, err := json.Marshal(&base64EncodedStoreMap)
	if err != nil {
		return err
	}
	dataFileName := directoryName + "/data.json"
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
