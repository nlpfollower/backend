package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type TestServer struct {
	Server *Server
	URL    string
}

func NewTestServer(t *testing.T) *TestServer {
	tempDir, err := os.MkdirTemp("", "deltamind-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	server, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Use httptest.NewServer to create a test server
	testServer := httptest.NewServer(server.router)

	return &TestServer{
		Server: server,
		URL:    testServer.URL,
	}
}

func (ts *TestServer) Close() {
	// Close the database manager
	ts.Server.dbManager.Close()

	// Remove the temporary directory
	os.RemoveAll(ts.Server.dbManager.GetDBPath())
}

func performRequest[Req any, Resp any](t *testing.T, ts *TestServer, method, path string, req Req) (*Resp, error) {
	url := fmt.Sprintf("%s%s", ts.URL, path)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	httpReq, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response Resp
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}
