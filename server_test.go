package main

import (
	"io"
	"net/http/httptest"
	"testing"
)

func TestJSON(test *testing.T) {
	testCases := []struct {
		input  map[string]string
		output string
	}{
		{map[string]string{"message": "Hello world"}, `{"message":"Hello world"}`},
	}
	responseRecorder := httptest.NewRecorder()
	JSON(responseRecorder, testCases[0].input)
	response := responseRecorder.Result()
	defer response.Body.Close()
	jsonGot, err := io.ReadAll(response.Body)
	if err != nil {
		test.Fatalf("Error reading response body: %s", err)
	}
	if string(jsonGot) != testCases[0].output {
		test.Errorf("Output: %s, expected: %s", jsonGot, testCases[0].output)
	}
}
