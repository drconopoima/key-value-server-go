package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cornelk/hashmap"
)

var directoryName string = "testdata"

func TestJSON(test *testing.T) {
	test.Parallel()
	header := http.Header{}
	headerKey := "Content-Type"
	headerValue := "application/json; charset=utf-8"
	header.Add(headerKey, headerValue)
	firstHashMap := &hashmap.HashMap{}
	firstHashMap.Set("message", "Hello world")
	secondHashMap := &hashmap.HashMap{}
	secondHashMap.Set("data", "tables testing")
	testCases := []struct {
		input  interface{}
		header http.Header
		output string
		status string
	}{
		{firstHashMap, header, `{"message":"Hello world"}`, "200 OK"},
		//{secondHashMap, header, `{"data":"tables testing"}}`, "200 OK"},
		//{make(chan bool), header, `{"error":"json: unsupported type: chan bool"}`, "500 Internal Server Error"},
	}
	for _, testCase := range testCases {
		responseRecorder := httptest.NewRecorder()
		var toJson interface{}
		hashm, ok := testCase.input.(*hashmap.HashMap)
		if ok {
			for item := range hashm.Iter() {
				key := item.Key.(string)
				value := item.Value.(string)
				toJson = map[string]string{key: value}
			}
		} else {
			toJson = testCase.input
		}

		JSON(responseRecorder, toJson.(map[string]string))
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
	keyValueStore := hashmap.HashMap{}
	keyValueStore.Set("key1", "value1")
	keyValueStore.Set("key3", "value3")
	base64EncodedStore := hashmap.HashMap{}
	for keyValueStruct := range keyValueStore.Iter() {
		encodedKey := base64.URLEncoding.EncodeToString([]byte(keyValueStruct.Key.(string)))
		encodedValue := base64.URLEncoding.EncodeToString([]byte(keyValueStruct.Value.(string)))
		base64EncodedStore.Set(encodedKey, encodedValue)
	}
	err := callLoadData(test, &base64EncodedStore)
	if err != nil {
		test.Fatalf("Couldn't load test data from file %v. Error: %v", directoryName+"/data.json", err)
	}
	value1Interface, _ := keyValueStore.Get("key1")
	// fmt.Println(value1Interface)
	value1, ok := value1Interface.(string)
	// fmt.Println(value1)
	if !ok {
		value1 = ""
	}
	value2Interface, _ := keyValueStore.Get("key2")
	// fmt.Println(value2Interface)
	value2, ok := value2Interface.(string)
	// fmt.Println(value2)
	if !ok {
		value2 = ""
	}
	value3Interface, _ := keyValueStore.Get("key3")
	// fmt.Println(value3Interface)
	value3, ok := value3Interface.(string)
	// fmt.Println(value3)
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
		fmt.Println(valueGot)
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

func callLoadData(testbench testing.TB, base64EncodedStore *hashmap.HashMap) error {
	makeStorage(testbench)
	// defer cleanupStorage(testbench, directoryName+"/data.json")
	var base64EncodedStoreMap map[string]string = map[string]string{}
	for keyValueStruct := range base64EncodedStore.Iter() {
		base64EncodedStoreMap[keyValueStruct.Key.(string)] = keyValueStruct.Value.(string)
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
	return loadData(dataFileName, base64Data, data)
}

func BenchmarkGet(bench *testing.B) {
	bench.ResetTimer()
	for i := 0; i < bench.N; i++ {
		Get(context.Background(), "key1")
	}
}
