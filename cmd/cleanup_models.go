package cmd

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/spf13/cobra"
	"time"
)

func NewCleanupDuplicateModelsCommand() *cobra.Command {
	var userEmail string
	var force bool

	cmd := &cobra.Command{
		Use:   "cleanup-duplicates",
		Short: "Clean up duplicate model entries for a user",
		Long:  "Remove duplicate model entries keeping only the latest version of each model",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return cleanupDuplicates(cfg.DBPath, userEmail, force)
		},
	}

	cmd.Flags().StringVar(&userEmail, "user", "", "User email (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.MarkFlagRequired("user")

	return cmd
}

func cleanupDuplicates(dbPath, userEmail string, force bool) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	userID := db.NewDigest(api.GetUserIDFromEmail(userEmail))

	// First, check what we have
	var modelCount int
	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Check if user exists
		user, err := txn.GetUser(userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %v", err)
		}
		if user == nil {
			return fmt.Errorf("user not found for email: %s", userEmail)
		}

		// Get model count
		models, err := txn.GetUserModels(userID, 10000, uint64(time.Now().UnixNano()))
		if err != nil {
			return fmt.Errorf("failed to get user models: %v", err)
		}

		modelCount = len(models)
		fmt.Printf("Found %d total model entries for user %s\n", modelCount, userEmail)

		// Count unique models
		uniqueIDs := make(map[string]bool)
		for _, model := range models {
			uniqueIDs[model.ID.String()] = true
		}

		fmt.Printf("Unique models: %d\n", len(uniqueIDs))
		fmt.Printf("Duplicate entries: %d\n", modelCount-len(uniqueIDs))

		return nil
	})

	if err != nil {
		return err
	}

	if !force {
		fmt.Printf("\nThis will clean up all duplicate model entries. Continue? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cleanup cancelled")
			return nil
		}
	}

	// Perform the cleanup
	var duplicatesRemoved int
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		removed, err := txn.CleanupDuplicateModels(userID)
		duplicatesRemoved = removed
		return err
	})

	if err != nil {
		return fmt.Errorf("error during cleanup: %v", err)
	}

	fmt.Printf("\nCleanup complete! Removed %d duplicate entries.\n", duplicatesRemoved)

	// Verify the result
	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		models, err := txn.GetUserModels(userID, 1000, uint64(time.Now().UnixNano()))
		if err != nil {
			return fmt.Errorf("failed to verify: %v", err)
		}

		fmt.Printf("User now has %d model entries\n", len(models))
		return nil
	})

	return err
}
