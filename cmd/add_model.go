package cmd

import (
	"encoding/hex"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/spf13/cobra"
	"time"
)

func NewAddBaseModelCommand() *cobra.Command {
	var modelName, modelSize, checkpointPath string

	cmd := &cobra.Command{
		Use:   "add-base-model",
		Short: "Add a base model (llama-8b or llama-70b)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return addBaseModel(cfg.DBPath, modelName, modelSize, checkpointPath)
		},
	}

	cmd.Flags().StringVar(&modelName, "name", "", "Model name (llama-8b or llama-70b)")
	cmd.Flags().StringVar(&modelSize, "size", "", "Model size (8B or 70B)")
	cmd.Flags().StringVar(&checkpointPath, "path", "", "Physical path to model")
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("size")
	cmd.MarkFlagRequired("path")

	return cmd
}

func addBaseModel(dbPath, modelName, modelSize, checkpointPath string) error {
	// Validate model name
	if modelName != "llama-8b" && modelName != "llama-70b" {
		return fmt.Errorf("invalid model name: %s (must be llama-8b or llama-70b)", modelName)
	}

	// Validate model size
	if modelSize != "8B" && modelSize != "70B" {
		return fmt.Errorf("invalid model size: %s (must be 8B or 70B)", modelSize)
	}

	// Create database manager
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	// Generate model ID directly from model name - same pattern as WebSocket handler
	modelID := db.NewDigest([]byte(modelName))

	// Create a special system user ID for base models
	systemUserID := db.NewDigest([]byte("system-base-models"))

	// Create the base model
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		// Check if model already exists
		existingModel, _ := txn.GetModel(modelID)
		if existingModel != nil {
			return fmt.Errorf("model %s already exists", modelName)
		}

		timeNow := time.Now()
		model := storage.ModelInfo{
			ID:             modelID,
			UserID:         systemUserID,
			Name:           modelName,
			DisplayName:    modelName,
			ModelType:      storage.ModelTypeBase,
			BaseModel:      modelName,
			ModelSize:      modelSize,
			Status:         storage.ModelStatusReady,
			CheckpointPath: checkpointPath,
			CreatedAt:      timeNow,
			UpdatedAt:      timeNow,
		}

		if err := txn.SetModel(systemUserID, &model); err != nil {
			return fmt.Errorf("failed to create base model: %v", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Base model %s added successfully\n", modelName)
	fmt.Printf("  ID: %s\n", modelID.String())
	fmt.Printf("  Path: %s\n", checkpointPath)
	return nil
}

func NewListModelsCommand() *cobra.Command {
	var showAll bool
	var userEmail string

	cmd := &cobra.Command{
		Use:   "list-models",
		Short: "List models in the database",
		Long:  "List models in the database. By default shows only base models. Use --all to show all models or --user to show models for a specific user.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return listModels(cfg.DBPath, showAll, userEmail)
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Show all models from all users")
	cmd.Flags().StringVar(&userEmail, "user", "", "Show models for a specific user (by email)")

	return cmd
}

func listModels(dbPath string, showAll bool, userEmail string) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		if showAll {
			// Show all models - we'll need to iterate through all users
			fmt.Println("Listing all models:")
			fmt.Println("==================")

			// First show base models
			systemUserID := db.NewDigest([]byte("system-base-models"))
			baseModels, err := txn.GetUserModels(systemUserID, 100, uint64(time.Now().UnixNano()))
			if err != nil {
				return fmt.Errorf("failed to get base models: %v", err)
			}

			if len(baseModels) > 0 {
				fmt.Println("\nBase Models:")
				for _, model := range baseModels {
					printModelInfo(model)
				}
			}

			// Note: To show ALL models, you'd need to iterate through all users
			// This would require adding a method to list all users first
			fmt.Println("\n(Note: Currently only showing base models. Full user iteration not implemented)")

		} else if userEmail != "" {
			// Show models for specific user
			userID := db.NewDigest(api.GetUserIDFromEmail(userEmail))

			// First check if user exists
			user, err := txn.GetUser(userID)
			if err != nil {
				return fmt.Errorf("failed to get user: %v", err)
			}
			if user == nil {
				return fmt.Errorf("user not found for email: %s", userEmail)
			}

			// Get user's models
			models, err := txn.GetUserModels(userID, 100, uint64(time.Now().UnixNano()))
			if err != nil {
				return fmt.Errorf("failed to get user models: %v", err)
			}

			fmt.Printf("Models for user %s (%s):\n", user.Username, user.Email)
			fmt.Printf("Found %d models:\n", len(models))
			fmt.Println("=================")
			for _, model := range models {
				printModelInfo(model)
			}

		} else {
			// Default: show only base models
			systemUserID := db.NewDigest([]byte("system-base-models"))
			models, err := txn.GetUserModels(systemUserID, 100, uint64(time.Now().UnixNano()))
			if err != nil {
				return fmt.Errorf("failed to get models: %v", err)
			}

			fmt.Printf("Found %d base models:\n", len(models))
			fmt.Println("====================")
			for _, model := range models {
				printModelInfo(model)
			}
		}

		return nil
	})

	return err
}

func printModelInfo(model *storage.ModelInfo) {
	fmt.Printf("\nModel: %s\n", model.Name)
	fmt.Printf("  ID: %s\n", model.ID.String())
	fmt.Printf("  Display Name: %s\n", model.DisplayName)
	fmt.Printf("  Type: %s\n", model.ModelType)
	fmt.Printf("  Size: %s\n", model.ModelSize)
	fmt.Printf("  Status: %s\n", model.Status)
	fmt.Printf("  Path: %s\n", model.CheckpointPath)
	fmt.Printf("  Created: %s\n", model.CreatedAt.Format(time.RFC3339))
	if model.ParentID != nil {
		fmt.Printf("  Parent ID: %s\n", model.ParentID.String())
	}
	if model.TrainingJobID != "" {
		fmt.Printf("  Training Job: %s\n", model.TrainingJobID)
	}
}

func NewDeleteModelCommand() *cobra.Command {
	var modelID string
	var modelName string
	var userEmail string
	var allUserModels bool
	var force bool

	cmd := &cobra.Command{
		Use:   "delete-model",
		Short: "Delete a model or all models for a user from the database",
		Long:  "Delete a model by ID or name, or delete all models for a specific user. Use --force to skip confirmation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)

			// Check for mutually exclusive options
			optionCount := 0
			if modelID != "" {
				optionCount++
			}
			if modelName != "" {
				optionCount++
			}
			if allUserModels {
				optionCount++
			}

			if optionCount == 0 {
				return fmt.Errorf("must specify either --id, --name, or --all-user-models")
			}
			if optionCount > 1 {
				return fmt.Errorf("--id, --name, and --all-user-models are mutually exclusive")
			}

			if allUserModels {
				if userEmail == "" {
					return fmt.Errorf("--user is required when using --all-user-models")
				}
				return deleteAllUserModels(cfg.DBPath, userEmail, force)
			}

			return deleteModel(cfg.DBPath, modelID, modelName, force)
		},
	}

	cmd.Flags().StringVar(&modelID, "id", "", "Model ID (hex string)")
	cmd.Flags().StringVar(&modelName, "name", "", "Model name (for base models)")
	cmd.Flags().StringVar(&userEmail, "user", "", "User email (required with --all-user-models)")
	cmd.Flags().BoolVar(&allUserModels, "all-user-models", false, "Delete all models for the specified user")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")

	return cmd
}

func deleteModel(dbPath, modelID, modelName string, force bool) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	var modelDigest db.Digest
	var model *storage.ModelInfo

	// First, find the model
	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		if modelID != "" {
			// Delete by ID
			idBytes, err := hex.DecodeString(modelID)
			if err != nil {
				return fmt.Errorf("invalid model ID format: %v", err)
			}
			modelDigest = db.NewDigest(idBytes)
			model, err = txn.GetModel(modelDigest)
			if err != nil {
				return fmt.Errorf("failed to get model: %v", err)
			}
		} else {
			// Delete by name - just use the name as bytes for the digest
			modelDigest = db.NewDigest([]byte(modelName))
			model, err = txn.GetModel(modelDigest)
			if err != nil {
				return fmt.Errorf("failed to get model: %v", err)
			}
		}

		if model == nil {
			return fmt.Errorf("model not found")
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Show model info and confirm
	fmt.Println("Model to delete:")
	printModelInfo(model)

	if !force {
		fmt.Print("\nAre you sure you want to delete this model? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Delete the model
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		return txn.DeleteModel(modelDigest)
	})

	if err != nil {
		return fmt.Errorf("failed to delete model: %v", err)
	}

	fmt.Printf("Model %s (ID: %s) deleted successfully\n", model.Name, modelDigest.String())
	return nil
}

func deleteAllUserModels(dbPath, userEmail string, force bool) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	userID := db.NewDigest(api.GetUserIDFromEmail(userEmail))
	var modelsToDelete []*storage.ModelInfo

	// First, get all models for this user
	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Check if user exists
		user, err := txn.GetUser(userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %v", err)
		}
		if user == nil {
			return fmt.Errorf("user not found for email: %s", userEmail)
		}

		// Get all models for this user
		models, err := txn.GetUserModels(userID, 1000, uint64(time.Now().UnixNano()))
		if err != nil {
			return fmt.Errorf("failed to get user models: %v", err)
		}
		modelsToDelete = models
		return nil
	})

	if err != nil {
		return err
	}

	if len(modelsToDelete) == 0 {
		fmt.Printf("No models found for user %s\n", userEmail)
		return nil
	}

	// Show models and confirm
	fmt.Printf("Found %d models for user %s:\n", len(modelsToDelete), userEmail)
	fmt.Println("================================")

	// Group models by base name to show duplicates
	modelGroups := make(map[string][]*storage.ModelInfo)
	for _, model := range modelsToDelete {
		modelGroups[model.Name] = append(modelGroups[model.Name], model)
	}

	for modelName, models := range modelGroups {
		if len(models) == 1 {
			fmt.Printf("- %s (ID: %s)\n", modelName, models[0].ID.String())
		} else {
			fmt.Printf("- %s (%d duplicates)\n", modelName, len(models))
			for i, model := range models {
				fmt.Printf("  [%d] ID: %s, Updated: %s\n", i+1, model.ID.String(), model.UpdatedAt.Format(time.RFC3339))
			}
		}
	}

	if !force {
		fmt.Printf("\nAre you sure you want to delete ALL %d models for user %s? (y/N): ", len(modelsToDelete), userEmail)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Delete all models
	deletedCount := 0
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		for _, model := range modelsToDelete {
			if err := txn.DeleteModel(model.ID); err != nil {
				fmt.Printf("Warning: failed to delete model %s: %v\n", model.Name, err)
			} else {
				deletedCount++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error during deletion: %v", err)
	}

	fmt.Printf("\nSuccessfully deleted %d models for user %s\n", deletedCount, userEmail)
	return nil
}

func NewDeleteAllModelsCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete-all-models",
		Short: "Delete all base models from the database",
		Long:  "Delete all base models. This is useful for cleaning up models with random IDs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return deleteAllModels(cfg.DBPath, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")

	return cmd
}

func deleteAllModels(dbPath string, force bool) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	var modelsToDelete []*storage.ModelInfo

	// First, get all base models
	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		systemUserID := db.NewDigest([]byte("system-base-models"))
		models, err := txn.GetUserModels(systemUserID, 1000, uint64(time.Now().UnixNano()))
		if err != nil {
			return fmt.Errorf("failed to get models: %v", err)
		}
		modelsToDelete = models
		return nil
	})

	if err != nil {
		return err
	}

	if len(modelsToDelete) == 0 {
		fmt.Println("No base models found")
		return nil
	}

	// Show models and confirm
	fmt.Printf("Found %d base models to delete:\n", len(modelsToDelete))
	fmt.Println("================================")
	for _, model := range modelsToDelete {
		fmt.Printf("- %s (ID: %s)\n", model.Name, model.ID.String())
	}

	if !force {
		fmt.Printf("\nAre you sure you want to delete ALL %d models? (y/N): ", len(modelsToDelete))
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Delete all models
	deletedCount := 0
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		for _, model := range modelsToDelete {
			if err := txn.DeleteModel(model.ID); err != nil {
				fmt.Printf("Warning: failed to delete model %s: %v\n", model.Name, err)
			} else {
				deletedCount++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error during deletion: %v", err)
	}

	fmt.Printf("\nSuccessfully deleted %d models\n", deletedCount)
	return nil
}
