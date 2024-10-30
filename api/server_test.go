package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

type TestServer struct {
	Server          *Server
	URL             string
	WsURL           string
	MockNexusServer *MockNexusServer
	httpServer      *httptest.Server
}

func NewTestServer(t *testing.T) *TestServer {
	tempDir, err := os.MkdirTemp("", "deltamind-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Start the mock Nexus server with port 0 (system assigned)
	mockNexus, err := NewMockNexusServer(0)
	if err != nil {
		t.Fatalf("Failed to create mock Nexus server: %v", err)
	}

	// Get the assigned port from the listener's address
	nexusPort := mockNexus.listener.Addr().(*net.TCPAddr).Port

	server, err := NewServer(tempDir, nexusPort)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start the NexusClient
	server.nexusClient.Start()

	// Use httptest.NewServer to create a test server
	httpServer := httptest.NewServer(server.router)

	// Create WebSocket URL
	wsURL := url.URL{Scheme: "ws", Host: httpServer.Listener.Addr().String(), Path: "/ws"}

	ts := &TestServer{
		Server:          server,
		URL:             httpServer.URL,
		WsURL:           wsURL.String(),
		MockNexusServer: mockNexus,
		httpServer:      httpServer,
	}

	// Wait for the NexusClient to establish a connection
	if err := ts.waitForNexusConnection(5 * time.Second); err != nil {
		t.Fatalf("NexusClient failed to connect: %v", err)
	}

	return ts
}

func (ts *TestServer) Close() {
	// Stop the NexusClient
	ts.Server.nexusClient.Stop()

	// Close the HTTP test server
	ts.httpServer.Close()

	// Close the database manager
	ts.Server.dbManager.Close()

	// Close the mock Nexus server
	ts.MockNexusServer.Close()

	// Remove the temporary directory
	os.RemoveAll(ts.Server.dbManager.GetDBPath())
}

func (ts *TestServer) waitForNexusConnection(timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for Nexus connection")
		}

		if ts.Server.nexusClient.isConnected {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
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
