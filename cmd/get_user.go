package cmd

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
)

func NewGetUserCommand() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "get-user",
		Short: "Retrieve user information by email",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return getUser(cfg.DBPath, email)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User's email address")
	cmd.MarkFlagRequired("email")

	return cmd
}

func getUser(dbPath, email string) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	userID := storage.NewDigest(api.GetUserIDFromEmail(email))

	user, err := dbManager.GetUser(userID)
	if err != nil {
		return fmt.Errorf("error retrieving user: %v", err)
	}

	if user == nil {
		return fmt.Errorf("user not found for email: %s", email)
	}

	fmt.Printf("User Information:\n")
	fmt.Printf("ID: %s\n", user.ID)
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Password (hash): %s\n", user.PasswordHash)
	fmt.Printf("Created At: %s\n", user.CreatedAt)

	return nil
}
