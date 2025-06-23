package cmd

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/spf13/cobra"
	"time"
)

func NewAddBaseModelCommand() *cobra.Command {
	var modelName, modelSize, physicalPath string

	cmd := &cobra.Command{
		Use:   "add-base-model",
		Short: "Add a base model (llama-8b or llama-70b)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return addBaseModel(cfg.DBPath, modelName, modelSize, physicalPath)
		},
	}

	cmd.Flags().StringVar(&modelName, "name", "", "Model name (llama-8b or llama-70b)")
	cmd.Flags().StringVar(&modelSize, "size", "", "Model size (8B or 70B)")
	cmd.Flags().StringVar(&physicalPath, "path", "", "Physical path to model")
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("size")
	cmd.MarkFlagRequired("path")

	return cmd
}

func addBaseModel(dbPath, modelName, modelSize, physicalPath string) error {
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

	// Generate model ID
	modelIDBytes, err := api.GenerateRandomBytes(32)
	if err != nil {
		return fmt.Errorf("failed to generate model ID: %v", err)
	}

	// Create a special system user ID for base models
	systemUserID := db.NewDigest([]byte("system-base-models"))

	// Create the base model
	err = dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		timeNow := time.Now()
		model := storage.ModelInfo{
			ID:           db.NewDigest(modelIDBytes),
			UserID:       systemUserID,
			Name:         modelName,
			DisplayName:  modelName,
			ModelType:    storage.ModelTypeBase,
			BaseModel:    modelName,
			ModelSize:    modelSize,
			Status:       storage.ModelStatusReady,
			PhysicalPath: physicalPath,
			CreatedAt:    timeNow,
			UpdatedAt:    timeNow,
		}

		if err := txn.SetModel(systemUserID, &model); err != nil {
			return fmt.Errorf("failed to create base model: %v", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Base model %s added successfully at path: %s\n", modelName, physicalPath)
	return nil
}

func NewListModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-models",
		Short: "List all models in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return listModels(cfg.DBPath)
		},
	}

	return cmd
}

func listModels(dbPath string) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	err = dbManager.View(func(txn *storage.DatabaseTransaction) error {
		// Get all models for system user
		systemUserID := db.NewDigest([]byte("system-base-models"))
		models, err := txn.GetUserModels(systemUserID, 100, uint64(time.Now().UnixNano()))
		if err != nil {
			return fmt.Errorf("failed to get models: %v", err)
		}

		fmt.Printf("Found %d base models:\n", len(models))
		for _, model := range models {
			fmt.Printf("- %s (%s) - Status: %s, Path: %s\n",
				model.Name, model.ModelSize, model.Status, model.PhysicalPath)
		}

		return nil
	})

	return err
}
