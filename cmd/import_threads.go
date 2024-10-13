package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"os"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
)

func NewImportThreadsCommand() *cobra.Command {
	var email, jsonFile string

	cmd := &cobra.Command{
		Use:   "import-threads",
		Short: "Import threads with messages from a JSON file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getConfig(cmd)
			return importThread(cfg.DBPath, email, jsonFile)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Email to associate with the threads")
	cmd.Flags().StringVar(&jsonFile, "json-file", "", "Path to the JSON file containing thread data")
	cmd.MarkFlagRequired("email")
	cmd.MarkFlagRequired("json-file")

	return cmd
}

func importThread(dbPath, email, jsonFile string) error {
	dbManager, err := storage.NewDatabaseManager(dbPath)
	if err != nil {
		return fmt.Errorf("error creating database manager: %v", err)
	}
	if err := dbManager.Setup(); err != nil {
		return fmt.Errorf("error setting up database: %v", err)
	}
	defer dbManager.Close()

	// Read and parse JSON file
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return fmt.Errorf("error reading JSON file: %v", err)
	}

	var jsonData struct {
		Threads map[string]ThreadData `json:"threads"`
	}

	if err := json.Unmarshal(data, &jsonData); err != nil {
		return fmt.Errorf("error parsing JSON: %v", err)
	}

	userID := storage.NewDigest(api.GetUserIDFromEmail(email))
	for _, threadData := range jsonData.Threads {
		fmt.Printf("Importing thread with title: %s, created_time: %f, updated_time: %f\n", threadData.Title, threadData.CreatedTime, threadData.UpdatedTime)
		// Generate a random thread ID
		threadIDBytes, err := api.GenerateRandomBytes(32) // 32 bytes for a 256-bit ID
		if err != nil {
			return fmt.Errorf("error generating thread ID: %v", err)
		}
		// Create thread
		newThread := &storage.Thread{
			ID:        storage.NewDigest(threadIDBytes),
			UserID:    userID,
			Title:     threadData.Title,
			CreatedAt: time.Unix(int64(threadData.CreatedTime), 0),
			UpdatedAt: time.Unix(int64(threadData.UpdatedTime), 0),
		}

		if err := dbManager.CreateThread(userID, newThread); err != nil {
			return fmt.Errorf("error creating thread: %v", err)
		}

		var parentID *storage.CompoundMessageID
		for _, chat := range threadData.Chats {
			// Create messages
			messageIDBytes, err := api.GenerateRandomBytes(32)
			if err != nil {
				return fmt.Errorf("error generating message ID: %v", err)
			}

			message := &storage.CompoundMessage{
				ID:        storage.NewDigest(messageIDBytes),
				ParentID:  parentID,
				ThreadID:  newThread.ID,
				Author:    chat.Role,
				Messages:  []string{chat.Message},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			parentID = &storage.CompoundMessageID{
				ID:        message.ID,
				MessageID: 0,
			}

			if err := dbManager.CreateMessage(newThread.ID, message); err != nil {
				return fmt.Errorf("error creating message: %v", err)
			}
		}

		fmt.Printf("Thread imported successfully. Thread ID: %s\n", newThread.ID)
	}

	return nil
}

type ThreadData struct {
	Title       string    `json:"title"`
	CreatedTime float64   `json:"created_time"`
	UpdatedTime float64   `json:"updated_time"`
	Chats       []Message `json:"chats"`
}

type Message struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}
