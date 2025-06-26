package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"github.com/stretchr/testify/require"
	"log"
	"net"
	"sync"
	"testing"
	"time"
)

type MockNexusServer struct {
	listener net.Listener
	conns    []net.Conn
	mu       sync.Mutex
}

func NewMockNexusServer(port int) (*MockNexusServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	server := &MockNexusServer{
		listener: listener,
		conns:    make([]net.Conn, 0),
	}

	go server.listen()

	return server, nil
}

func (s *MockNexusServer) listen() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()

		go s.handleConnection(conn)
	}
}

func (s *MockNexusServer) handleConnection(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req core.WrappedRequest
		err := decoder.Decode(&req)
		if err != nil {
			log.Printf("Error decoding request: %v", err)
			return
		}

		// Decode the inference request
		var inferReq core.InferenceRequest
		if err := json.Unmarshal(req.Data, &inferReq); err != nil {
			log.Printf("Error unmarshaling inference request: %v", err)
			continue
		}

		// Generate streaming responses
		partial := &core.InferenceResponse{
			Type:    core.ResponseTypePartial,
			Content: fmt.Sprintf("Partial response for request %s", req.RequestID),
			Status:  core.ResponseStatusSuccess,
		}

		final := &core.InferenceResponse{
			Type:    core.ResponseTypeFinal,
			Content: fmt.Sprintf("Final response for request %s", req.RequestID),
			Status:  core.ResponseStatusSuccess,
		}

		// Send partial response
		partialWrapped, err := core.NewWrappedResponse(req.RequestID, partial)
		if err != nil {
			log.Printf("Error creating partial response: %v", err)
			continue
		}
		if err := encoder.Encode(partialWrapped); err != nil {
			log.Printf("Error encoding partial response: %v", err)
			return
		}

		// Small delay to simulate processing
		time.Sleep(50 * time.Millisecond)

		// Send final response
		finalWrapped, err := core.NewWrappedResponse(req.RequestID, final)
		if err != nil {
			log.Printf("Error creating final response: %v", err)
			continue
		}
		if err := encoder.Encode(finalWrapped); err != nil {
			log.Printf("Error encoding final response: %v", err)
			return
		}
	}
}

func (s *MockNexusServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.conns {
		conn.Close()
	}

	return s.listener.Close()
}

func TestNexusClient(t *testing.T) {
	// Create client
	client := NewNexusClient(8081)
	client.Start()
	defer client.Stop()

	// Wait for client to connect
	time.Sleep(100 * time.Millisecond)

	t.Run("Basic Inference Request", func(t *testing.T) {
		userID := db.NewDigest([]byte("test-user"))
		modelID := "gpt-4"
		messages := []core.Message{
			{Role: "user", Content: "Hello"},
		}
		checkpointPath := "/mnt/cold/contents/dcp/llama-8b/checkpoint" // Add checkpoint path

		respChan, err := client.EnqueueInference(userID, modelID, messages, checkpointPath)
		require.NoError(t, err)

		// Collect responses
		var responses []*core.WrappedResponse
		for resp := range respChan {
			responses = append(responses, resp)
		}

		// Verify we got both partial and final responses
		require.GreaterOrEqual(t, len(responses), 5)

		// Check partial response
		var partial core.InferenceResponse
		err = json.Unmarshal(responses[0].Data, &partial)
		require.NoError(t, err)
		require.Equal(t, core.ResponseTypePartial, partial.Type)
		require.Equal(t, core.ResponseStatusSuccess, partial.Status)

		// Check final response
		var final core.InferenceResponse
		err = json.Unmarshal(responses[len(responses)-1].Data, &final)
		require.NoError(t, err)
		require.Equal(t, core.ResponseTypeFinal, final.Type)
		require.Equal(t, core.ResponseStatusSuccess, final.Status)
	})
}
