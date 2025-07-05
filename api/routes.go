package api

import (
	"github.com/gorilla/mux"
	"github.com/nlpfollower/deltamind/backend/storage"
)

type APIRouter struct {
	dbManager   *storage.DatabaseManager
	nexusClient *NexusClient
}

func NewAPIRouter(dbManager *storage.DatabaseManager, nexusClient *NexusClient) *APIRouter {
	return &APIRouter{
		dbManager:   dbManager,
		nexusClient: nexusClient,
	}
}

func (router *APIRouter) SetupRoutes(mux *mux.Router) {
	// User routes
	mux.HandleFunc("/api/v0/sign-up", router.SignUp).Methods("POST")
	mux.HandleFunc("/api/v0/sign-in", router.SignIn).Methods("POST")

	// Space routes
	mux.HandleFunc("/api/v0/create-space", router.CreateSpace).Methods("POST")
	mux.HandleFunc("/api/v0/get-spaces", router.GetSpaces).Methods("POST")
	mux.HandleFunc("/api/v0/delete-space", router.DeleteSpace).Methods("POST")
	mux.HandleFunc("/api/v0/update-space", router.UpdateSpace).Methods("POST")

	// Thread routes
	mux.HandleFunc("/api/v0/create-thread", router.CreateThread).Methods("POST")
	mux.HandleFunc("/api/v0/get-threads", router.GetThreads).Methods("POST")
	mux.HandleFunc("/api/v0/delete-thread", router.DeleteThread).Methods("POST")
	mux.HandleFunc("/api/v0/update-thread", router.UpdateThread).Methods("POST")

	// Message routes
	mux.HandleFunc("/api/v0/create-message", router.CreateMessage).Methods("POST")
	mux.HandleFunc("/api/v0/get-messages", router.GetMessages).Methods("POST")

	// Model routes
	mux.HandleFunc("/api/v0/create-model", router.CreateModel).Methods("POST")
	mux.HandleFunc("/api/v0/get-models", router.GetModels).Methods("POST")
	mux.HandleFunc("/api/v0/clone-model", router.CloneModel).Methods("POST")

	// Model Iteration routes
	//mux.HandleFunc("/api/v0/create-model-iteration", router.CreateModelIteration).Methods("POST")
	//mux.HandleFunc("/api/v0/get-model-iterations", router.GetModelIterations).Methods("POST")

	// Session management routes
	mux.HandleFunc("/api/v0/session/start", router.StartSession).Methods("POST")
	mux.HandleFunc("/api/v0/session/status", router.GetSessionStatus).Methods("POST")
	mux.HandleFunc("/api/v0/session/extend", router.ExtendSession).Methods("POST")
	mux.HandleFunc("/api/v0/session/stop", router.StopSession).Methods("POST")

	// Training routes
	mux.HandleFunc("/api/v0/training/start", router.StartTraining).Methods("POST")
	mux.HandleFunc("/api/v0/training/status", router.GetTrainingStatus).Methods("POST")

	// System status route
	mux.HandleFunc("/api/v0/system/status", router.GetSystemStatus).Methods("POST")
}
