package api

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/nlpfollower/deltamind/nexus/core"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type NexusClient struct {
	port           int
	conn           net.Conn
	mu             sync.Mutex
	isConnected    bool
	requestQueue   chan *core.WrappedRequest
	stopChan       chan struct{}
	reconnectDelay time.Duration
	requestMap     map[string]chan *core.WrappedResponse
	requestMapMu   sync.Mutex
}

func NewNexusClient(port int) *NexusClient {
	return &NexusClient{
		port:           port,
		requestQueue:   make(chan *core.WrappedRequest, 100),
		stopChan:       make(chan struct{}),
		reconnectDelay: 1 * time.Second,
		requestMap:     make(map[string]chan *core.WrappedResponse),
	}
}

func (nc *NexusClient) Start() {
	go nc.handleConnection()
	go nc.handleOutboundMessages()
}

func (nc *NexusClient) Stop() {
	close(nc.stopChan)
}

func (nc *NexusClient) handleConnection() {
	for {
		select {
		case <-nc.stopChan:
			return
		default:
			if err := nc.connect(); err != nil {
				log.Printf("Failed to connect to Nexus server: %v. Retrying in %v", err, nc.reconnectDelay)
				time.Sleep(nc.reconnectDelay)
				continue
			}
			nc.handleInboundMessages()
		}
	}
}

func (nc *NexusClient) connect() error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.isConnected {
		return nil
	}

	conn, err := net.Dial("tcp", fmt.Sprintf(":%d", nc.port))
	if err != nil {
		return fmt.Errorf("failed to connect to nexus server: %w", err)
	}

	nc.conn = conn
	nc.isConnected = true
	return nil
}

func (nc *NexusClient) disconnect() {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.conn != nil {
		nc.conn.Close()
	}
	nc.isConnected = false
}

func (nc *NexusClient) handleInboundMessages() {
	defer nc.disconnect()

	decoder := json.NewDecoder(nc.conn)
	for {
		select {
		case <-nc.stopChan:
			return
		default:
			var response core.WrappedResponse
			err := decoder.Decode(&response)
			if err != nil {
				if err != io.EOF {
					log.Printf("Error receiving message: %v", err)
				}
				return
			}
			nc.routeResponse(&response)
		}
	}
}

func (nc *NexusClient) routeResponse(response *core.WrappedResponse) {
	nc.requestMapMu.Lock()
	defer nc.requestMapMu.Unlock()

	if ch, ok := nc.requestMap[response.RequestID.String()]; ok {
		ch <- response
		if nc.isResponseFinal(response) {
			close(ch)
			delete(nc.requestMap, response.RequestID.String())
		}
	} else {
		log.Printf("Received response for unknown request ID: %s", response.RequestID)
	}
}

func (nc *NexusClient) handleOutboundMessages() {
	for {
		select {
		case <-nc.stopChan:
			return
		case req := <-nc.requestQueue:
			nc.mu.Lock()
			isConnected := nc.isConnected
			nc.mu.Unlock()

			if !isConnected {
				log.Println("Not connected to Nexus server. Retrying request later.")
				nc.requestQueue <- req
				time.Sleep(nc.reconnectDelay)
				continue
			}

			err := json.NewEncoder(nc.conn).Encode(req)
			if err != nil {
				log.Printf("Error sending request: %v", err)
				nc.requestQueue <- req
				time.Sleep(nc.reconnectDelay)
			}
		}
	}
}

func (nc *NexusClient) EnqueueTraining(trainReq *core.TrainingRequest) (<-chan *core.WrappedResponse, error) {
	responseChan := make(chan *core.WrappedResponse, 1)
	requestID := db.NewDigest([]byte(fmt.Sprintf("train-%d", time.Now().UnixNano())))

	wrappedReq, err := core.NewWrappedRequest(requestID, trainReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create training request: %w", err)
	}

	nc.requestMapMu.Lock()
	nc.requestMap[requestID.String()] = responseChan
	nc.requestMapMu.Unlock()

	nc.requestQueue <- wrappedReq
	return responseChan, nil
}

func (nc *NexusClient) GetTrainingStatus(statusReq *core.TrainingStatusRequest) (<-chan *core.WrappedResponse, error) {
	responseChan := make(chan *core.WrappedResponse, 1)
	requestID := db.NewDigest([]byte(fmt.Sprintf("train-status-%d", time.Now().UnixNano())))

	wrappedReq, err := core.NewWrappedRequest(requestID, statusReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create training status request: %w", err)
	}

	nc.requestMapMu.Lock()
	nc.requestMap[requestID.String()] = responseChan
	nc.requestMapMu.Unlock()

	nc.requestQueue <- wrappedReq
	return responseChan, nil
}

func (nc *NexusClient) EnqueueInference(userID db.Digest, modelID string, messages []core.Message, checkpointPath string, modelSize string) (<-chan *core.WrappedResponse, error) {
	responseChan := make(chan *core.WrappedResponse, 10) // Buffer for streaming responses
	requestID := db.NewDigest([]byte(fmt.Sprintf("req-%d", time.Now().UnixNano())))

	inferReq := &core.InferenceRequest{
		UserID:         userID,
		ModelID:        modelID,
		Messages:       messages,
		CheckpointPath: checkpointPath,
		ModelSize:      modelSize,
	}

	wrappedReq, err := core.NewWrappedRequest(requestID, inferReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	nc.requestMapMu.Lock()
	nc.requestMap[requestID.String()] = responseChan
	nc.requestMapMu.Unlock()

	nc.requestQueue <- wrappedReq
	return responseChan, nil
}

func (nc *NexusClient) EnqueueSession(sessionReq *core.SessionRequest) (<-chan *core.WrappedResponse, error) {
	responseChan := make(chan *core.WrappedResponse, 1) // Buffer for single response
	requestID := db.NewDigest([]byte(fmt.Sprintf("session-req-%d", time.Now().UnixNano())))

	wrappedReq, err := core.NewWrappedRequest(requestID, sessionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	nc.requestMapMu.Lock()
	nc.requestMap[requestID.String()] = responseChan
	nc.requestMapMu.Unlock()

	nc.requestQueue <- wrappedReq
	return responseChan, nil
}

// Update isResponseFinal to handle training responses
func (nc *NexusClient) isResponseFinal(response *core.WrappedResponse) bool {
	// Try inference response first
	var inferResp core.InferenceResponse
	if err := json.Unmarshal(response.Data, &inferResp); err == nil {
		return inferResp.Type == core.ResponseTypeFinal
	}

	// Session responses are always final
	var sessionResp core.SessionResponse
	if err := json.Unmarshal(response.Data, &sessionResp); err == nil {
		return true
	}

	// Training responses are always final
	var trainResp core.TrainingResponse
	if err := json.Unmarshal(response.Data, &trainResp); err == nil {
		return true
	}

	// Training status responses are always final
	var trainStatusResp core.TrainingStatusResponse
	if err := json.Unmarshal(response.Data, &trainStatusResp); err == nil {
		return true
	}

	// Default to true for unknown types
	return true
}
