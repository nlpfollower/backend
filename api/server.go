package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/nlpfollower/deltamind/backend/storage"
	"golang.org/x/net/websocket"
)

type Server struct {
	router           *mux.Router
	dbManager        *storage.DatabaseManager
	apiRouter        *APIRouter
	httpSrv          *http.Server
	nexusClient      *NexusClient
	webSocketHandler *WebSocketHandler
}

func NewServer(dbPath string, nexusPort int) (*Server, error) {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return nil, fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return nil, fmt.Errorf("error setting up database: %v", err)
	}

	nexusClient := NewNexusClient(nexusPort)

	s := &Server{
		router:      mux.NewRouter(),
		dbManager:   dbManager,
		apiRouter:   NewAPIRouter(dbManager, nexusClient),
		nexusClient: nexusClient,
	}

	s.webSocketHandler = NewWebSocketHandler(nexusClient, s.dbManager)

	s.apiRouter.SetupRoutes(s.router)
	s.setupWebSocket()

	// Apply logging middleware to all routes
	s.router.Use(LoggingHandler)

	return s, nil
}

func (s *Server) setupWebSocket() {
	s.router.Handle("/ws", websocket.Handler(s.webSocketHandler.HandleWebSocket))
}

func (s *Server) Start() error {
	s.nexusClient.Start()

	s.httpSrv = &http.Server{
		Addr:    ":8080",
		Handler: s.router,
	}

	go func() {
		log.Println("Server starting on :8080")
		log.Println("Request logging enabled - all incoming requests will be printed to console")
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	return s.waitForShutdown()
}

func (s *Server) waitForShutdown() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Println("Server is shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.nexusClient.Stop()

	if err := s.httpSrv.Shutdown(ctx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	if err := s.dbManager.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	log.Println("Server stopped")
	return nil
}
