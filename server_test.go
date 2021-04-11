package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
			test.Fatalf("Error reading response body: %s", err)
		}
		if string(jsonGot) != testCase.output {
			test.Errorf("Output: %s, expected: %s", jsonGot, testCase.output)
		}
		if contentType := response.Header.Get(headerKey); contentType != headerValue {
			test.Errorf("Output: %s, expected: %s", contentType, headerValue)
		}
		if status, testStatus := response.Status, testCase.status; status != testStatus {
			test.Errorf("Output: %s, expected: %s", status, testStatus)
		}
	}
}
