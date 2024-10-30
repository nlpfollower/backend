package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/spf13/cobra"
	"net/http"
	"net/http/httptest"
	"time"
)

func NewGetSpacesCommand() *cobra.Command {
	var email string
	var pageSize int
	var maxTimestamp uint64

	cmd := &cobra.Command{
		Use:   "get-spaces",
		Short: "Retrieve paginated spaces for a user by email",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return getSpaces(cfg.DBPath, email, pageSize, maxTimestamp)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User's email address")
	cmd.Flags().IntVar(&pageSize, "page-size", 10, "Number of spaces to fetch per page")
	cmd.Flags().Uint64Var(&maxTimestamp, "max-timestamp", 0, "Max timestamp for pagination (0 for latest)")
	cmd.MarkFlagRequired("email")

	return cmd
}

func getSpaces(dbPath, email string, pageSize int, maxTimestamp uint64) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	apiRouter := api.NewAPIRouter(dbManager)

	userID := db.NewDigest(api.GetUserIDFromEmail(email))
	authToken, err := api.GenerateAuthToken(userID.String())
	if err != nil {
		return fmt.Errorf("error generating auth token: %v", err)
	}

	requestBody := api.GetSpacesRequest{
		Limit:        pageSize,
		MaxTimestamp: maxTimestamp,
		AuthToken:    *authToken,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("error marshaling request body: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/v0/get-spaces", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	apiRouter.GetSpaces(rr, req)

	if rr.Code != http.StatusOK {
		return fmt.Errorf("get spaces failed with status code: %d, message: %s", rr.Code, rr.Body.String())
	}

	var response api.GetSpacesResponse
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		return fmt.Errorf("error unmarshaling response: %v", err)
	}

	fmt.Printf("Spaces for user %s:\n", email)
	for _, space := range response.Spaces {
		fmt.Printf("ID: %s\n", space.ID)
		fmt.Printf("Name: %s\n", space.Name)
		fmt.Printf("Description: %s\n", space.Description)
		fmt.Printf("Created At: %s\n", space.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Updated At: %s\n", space.UpdatedAt.Format(time.RFC3339))
		fmt.Println("---")
	}

	if response.NextMaxTimestamp != nil {
		fmt.Printf("Next max timestamp: %d\n", *response.NextMaxTimestamp)
		fmt.Println("To fetch the next page, use this timestamp with the --max-timestamp flag")
	} else {
		fmt.Println("No more spaces to fetch")
	}

	return nil
}
