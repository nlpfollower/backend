package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/nlpfollower/deltamind/backend/api"
	"github.com/nlpfollower/deltamind/database/db"
	"os"
	"time"

	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/spf13/cobra"
)

// OpenAIThread represents the OpenAI chat export format
type OpenAIThread struct {
	Title       string              `json:"title"`
	CreateTime  float64             `json:"create_time"`
	UpdateTime  float64             `json:"update_time"`
	Mapping     map[string]NodeData `json:"mapping"`
	CurrentNode string              `json:"current_node"`
}

type NodeData struct {
	ID       string       `json:"id"`
	Message  *MessageData `json:"message"`
	Parent   string       `json:"parent"`
	Children []string     `json:"children"`
}

type MessageData struct {
	Author  Author  `json:"author"`
	Content Content `json:"content"`
}

type Author struct {
	Role string `json:"role"`
}

type Content struct {
	ContentType string        `json:"content_type"`
	Parts       []interface{} `json:"parts"`
}

func getMessageContent(parts []interface{}) string {
	if len(parts) == 0 {
		return ""
	}

	// Try to get the first part as a string
	switch v := parts[0].(type) {
	case string:
		return v
	case map[string]interface{}:
		// If it's an object, try to get the text value
		if text, ok := v["text"].(string); ok {
			return text
		}
		// You might want to handle other cases or add more specific handling
	}
	return ""
}

func extractMessages(thread *OpenAIThread) []Message {
	var messages []Message
	currentID := thread.CurrentNode

	// Create a visited map to prevent infinite loops
	visited := make(map[string]bool)

	// Walk backwards through the message chain until we hit the root
	for currentID != "" && !visited[currentID] {
		visited[currentID] = true

		if node, exists := thread.Mapping[currentID]; exists && node.Message != nil {
			// Only add messages that have content
			content := getMessageContent(node.Message.Content.Parts)
			if content != "" {
				messages = append([]Message{{
					Role:    node.Message.Author.Role,
					Message: content,
				}}, messages...)
			}
			currentID = node.Parent
		} else {
			break
		}
	}

	return messages
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

	var openAIThreads []OpenAIThread
	if err := json.Unmarshal(data, &openAIThreads); err != nil {
		return fmt.Errorf("error parsing JSON: %v", err)
	}

	userID := db.NewDigest(api.GetUserIDFromEmail(email))

	// Create a "ChatGPT" space
	spaceIDBytes, err := api.GenerateRandomBytes(32)
	if err != nil {
		return fmt.Errorf("error generating space ID: %v", err)
	}
	space := &storage.Space{
		ID:          db.NewDigest(spaceIDBytes),
		UserID:      userID,
		Name:        "ChatGPT",
		Description: "Imported ChatGPT conversations",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		if err := txn.SetSpace(userID, space); err != nil {
			return fmt.Errorf("error creating space: %v", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error creating space: %v", err)
	}

	fmt.Printf("Created space 'ChatGPT' with ID: %s\n", space.ID)

	if err := dbManager.Update(func(txn *storage.DatabaseTransaction) error {
		for _, openAIThread := range openAIThreads {
			fmt.Printf("Importing thread with title: %s, created_time: %f, updated_time: %f\n",
				openAIThread.Title, openAIThread.CreateTime, openAIThread.UpdateTime)

			threadIDBytes, err := api.GenerateRandomBytes(32)
			if err != nil {
				return fmt.Errorf("error generating thread ID: %v", err)
			}

			newThread := &storage.Thread{
				ID:        db.NewDigest(threadIDBytes),
				SpaceID:   space.ID,
				Title:     openAIThread.Title,
				CreatedAt: time.Unix(int64(openAIThread.CreateTime), 0),
				UpdatedAt: time.Unix(int64(openAIThread.UpdateTime), 0),
			}

			if err := txn.SetThread(space.ID, newThread); err != nil {
				return fmt.Errorf("error creating thread: %v", err)
			}

			messages := extractMessages(&openAIThread)
			var parentID *storage.CompoundMessageID

			for _, chat := range messages {
				messageIDBytes, err := api.GenerateRandomBytes(32)
				if err != nil {
					return fmt.Errorf("error generating message ID: %v", err)
				}

				message := &storage.CompoundMessage{
					ID:        db.NewDigest(messageIDBytes),
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

				if err := txn.SetMessage(newThread.ID, message); err != nil {
					return fmt.Errorf("error creating message: %v", err)
				}
			}

			fmt.Printf("Thread imported successfully. Thread ID: %s\n", newThread.ID)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error setting user space: %v", err)
	}

	return nil
}

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

type Message struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}
