package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/database/db"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
)

func NewGetThreadsCommand() *cobra.Command {
	var email string
	var spaceID string
	var pageSize int
	var maxTimestamp uint64

	cmd := &cobra.Command{
		Use:   "get-threads",
		Short: "Retrieve paginated threads for a user by email within a specific space",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return getThreads(cfg.DBPath, email, spaceID, pageSize, maxTimestamp)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User's email address")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "ID of the space to search threads in")
	cmd.Flags().IntVar(&pageSize, "page-size", 10, "Number of threads to fetch per page")
	cmd.Flags().Uint64Var(&maxTimestamp, "max-timestamp", 0, "Max timestamp for pagination (0 for latest)")
	cmd.MarkFlagRequired("email")
	cmd.MarkFlagRequired("space-id")

	return cmd
}

func getThreads(dbPath, email, spaceIDStr string, pageSize int, maxTimestamp uint64) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	nexusClient := api.NewNexusClient(8081)

	apiRouter := api.NewAPIRouter(dbManager, nexusClient)

	userID := db.NewDigest(api.GetUserIDFromEmail(email))
	authToken, err := api.GenerateAuthToken(userID.String())
	if err != nil {
		return fmt.Errorf("error generating auth token: %v", err)
	}

	spaceID, err := db.DigestFromString(spaceIDStr)
	if err != nil {
		return fmt.Errorf("invalid space ID: %v", err)
	}

	requestBody := api.GetThreadsRequest{
		SpaceID:      spaceID,
		Limit:        pageSize,
		MaxTimestamp: maxTimestamp,
		AuthToken:    *authToken,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request body: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/v0/get-threads", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	apiRouter.GetThreads(rr, req)

	if rr.Code != http.StatusOK {
		return fmt.Errorf("get threads failed with status code: %d, message: %s", rr.Code, rr.Body.String())
	}

	var response api.GetThreadsResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		return fmt.Errorf("error unmarshaling response: %v", err)
	}

	fmt.Printf("Threads for user %s in space %s:\n", email, spaceIDStr)
	for _, thread := range response.Threads {
		fmt.Printf("ID: %s\n", thread.ID)
		fmt.Printf("Title: %s\n", thread.Title)
		fmt.Printf("Created At: %s\n", thread.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Updated At: %s\n", thread.UpdatedAt.Format(time.RFC3339))
		fmt.Println("---")
	}

	if response.NextMaxTimestamp != nil {
		fmt.Printf("Next max timestamp: %d\n", *response.NextMaxTimestamp)
		fmt.Println("To fetch the next page, use this timestamp with the --max-timestamp flag")
	} else {
		fmt.Println("No more threads to fetch")
	}

	return nil
}
