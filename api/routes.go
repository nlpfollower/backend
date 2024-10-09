package api

import (
	"github.com/gorilla/mux"
	"github.com/nlpfollower/deltamind/backend/storage"
)

type APIRouter struct {
	dbManager *storage.DatabaseManager
}

func NewAPIRouter(dbManager *storage.DatabaseManager) *APIRouter {
	return &APIRouter{dbManager: dbManager}
}

func (router *APIRouter) SetupRoutes(mux *mux.Router) {
	// User routes
	mux.HandleFunc("/v0/sign-up", router.SignUp).Methods("POST")
	mux.HandleFunc("/v0/sign-in", router.SignIn).Methods("POST")

	// Thread routes
	mux.HandleFunc("/v0/create-thread", router.CreateThread).Methods("POST")
	mux.HandleFunc("/v0/get-threads", router.GetThreads).Methods("POST")

	// Message routes
	mux.HandleFunc("/v0/create-message", router.CreateMessage).Methods("POST")
	mux.HandleFunc("/v0/get-messages", router.GetMessages).Methods("POST")

	// TODO: Add more routes as needed
}
