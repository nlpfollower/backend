package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
)

func NewAddUserCommand() *cobra.Command {
	var email, username, password string

	cmd := &cobra.Command{
		Use:   "add-user",
		Short: "Add a new user",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return addUser(cfg.DBPath, email, username, password)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User's email address")
	cmd.Flags().StringVar(&username, "username", "", "User's username")
	cmd.Flags().StringVar(&password, "password", "", "User's password")
	cmd.MarkFlagRequired("email")
	cmd.MarkFlagRequired("username")
	cmd.MarkFlagRequired("password")

	return cmd
}

func addUser(dbPath, email, username, password string) error {
	// Create a new database manager
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	nexusClient := api.NewNexusClient(8081)
	
	// Create a new API router
	apiRouter := api.NewAPIRouter(dbManager, nexusClient)

	// Create a mock HTTP request
	passwordHash := api.HashPassword(password)
	passwordHex := hex.EncodeToString(passwordHash)
	signUpReq := api.SignUpRequest{
		Email:       email,
		Username:    username,
		PasswordHex: passwordHex,
		Method:      storage.AuthMethodEmailPassword,
	}
	reqBody, err := json.Marshal(signUpReq)
	if err != nil {
		return fmt.Errorf("error marshaling sign-up request: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/v0/sign-up", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Create a mock HTTP response recorder
	rr := httptest.NewRecorder()

	// Call the SignUp handler
	apiRouter.SignUp(rr, req)

	// Check the response
	if rr.Code != http.StatusOK {
		return fmt.Errorf("sign-up failed with status code: %d, message: %s", rr.Code, rr.Body.String())
	}

	var response api.SignUpResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		return fmt.Errorf("error unmarshaling sign-up response: %v", err)
	}

	fmt.Printf("User created successfully. Auth Token: %s\n", response.AuthToken.SessionKey)
	return nil
}
