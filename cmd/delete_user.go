package cmd

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
)

func NewDeleteUserCommand() *cobra.Command {
	var email string

	cmd := &cobra.Command{
		Use:   "delete-user",
		Short: "Delete a user by email",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return deleteUser(cfg.DBPath, email)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "User's email address")
	cmd.MarkFlagRequired("email")

	return cmd
}

func deleteUser(dbPath, email string) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	userID := storage.NewDigest(api.GetUserIDFromEmail(email))
	if err := dbManager.DeleteUser(userID); err != nil {
		return fmt.Errorf("error deleting user: %v", err)
	}

	fmt.Printf("User with email %s has been successfully deleted.\n", email)
	return nil
}
