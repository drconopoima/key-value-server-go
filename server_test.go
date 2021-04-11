package main

import (
	"io"
	"net/http/httptest"
	"testing"
)

func TestJSON(test *testing.T) {
	input := map[string]map[string]string{"data": {"message": "Hello world"}}
	output := `{"data":{"message":"Hello world"}}`
	responseRecorder := httptest.NewRecorder()
	JSON(responseRecorder, input)
	response := responseRecorder.Result()
	defer response.Body.Close()
	jsonGot, err := io.ReadAll(response.Body)
	if err != nil {
		test.Fatalf("Error reading response body: %s", err)
	}
	if string(jsonGot) != output {
		test.Errorf("Output: %s, expected: %s", jsonGot, output)
	}
}
