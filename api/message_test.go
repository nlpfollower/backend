package api

import (
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMessageCreationAndRetrieval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user
	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)

	// Create a thread
	thread, err := createTestThread(t, ts, user, "Test Thread")
	require.NoError(t, err)

	// Create 10 messages in the thread
	createdMessages := createTestMessages(t, ts, user, thread, 10)

	// Test message retrieval
	testMessageRetrieval(t, ts, user, thread, createdMessages)

	// Test pagination
	testMessagePagination(t, ts, user, thread)

	// Test creating a message with a parent
	testCreateMessageWithParent(t, ts, user, thread, createdMessages[0])
}

func createTestThread(t *testing.T, ts *TestServer, user *SignUpResponse, title string) (*storage.Thread, error) {
	createThreadReq := CreateThreadRequest{
		Title:     title,
		AuthToken: user.AuthToken,
	}
	createThreadResp, err := performRequest[CreateThreadRequest, CreateThreadResponse](t, ts, "POST", "/v0/create-thread", createThreadReq)
	require.NoError(t, err)
	return &createThreadResp.Thread, nil
}

func createTestMessages(t *testing.T, ts *TestServer, user *SignUpResponse, thread *storage.Thread, count int) []*storage.CompoundMessage {
	var messages []*storage.CompoundMessage
	for i := 0; i < count; i++ {
		createMessageReq := CreateMessageRequest{
			ThreadID:  thread.ID,
			Author:    "user",
			Content:   "Test message",
			AuthToken: user.AuthToken,
		}
		createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/v0/create-message", createMessageReq)
		require.NoError(t, err)
		require.NotNil(t, createMessageResp.Message)
		messages = append(messages, createMessageResp.Message)
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
	return messages
}

func testMessageRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, thread *storage.Thread, createdMessages []*storage.CompoundMessage) {
	getMessagesReq := GetMessagesRequest{
		ThreadID:     thread.ID,
		Limit:        len(createdMessages),
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()), // Use a future timestamp to get all messages
		AuthToken:    user.AuthToken,
	}
	getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/v0/get-messages", getMessagesReq)
	require.NoError(t, err)
	require.Len(t, getMessagesResp.Messages, len(createdMessages))

	// Verify that all messages are retrieved in reverse order
	for i, message := range getMessagesResp.Messages {
		require.Equal(t, createdMessages[len(createdMessages)-1-i].ID, message.ID)
		require.Equal(t, createdMessages[len(createdMessages)-1-i].Author, message.Author)
		require.Equal(t, createdMessages[len(createdMessages)-1-i].Messages, message.Messages)
	}

	// Verify NextMaxTimestamp
	require.NotNil(t, getMessagesResp.NextMaxTimestamp)
	require.Equal(t, uint64(createdMessages[0].UpdatedAt.UnixNano()), *getMessagesResp.NextMaxTimestamp)
}

func testMessagePagination(t *testing.T, ts *TestServer, user *SignUpResponse, thread *storage.Thread) {
	pageSize := 3
	totalPages := 4

	var allMessages []*storage.CompoundMessage
	var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())

	for page := 0; page < totalPages; page++ {
		getMessagesReq := GetMessagesRequest{
			ThreadID:     thread.ID,
			Limit:        pageSize,
			MaxTimestamp: maxTimestamp,
			AuthToken:    user.AuthToken,
		}
		getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/v0/get-messages", getMessagesReq)
		require.NoError(t, err)

		if page < totalPages-1 {
			require.Len(t, getMessagesResp.Messages, pageSize)
		} else {
			// On the last page, we expect the remaining messages
			remainingMessages := 10 - (pageSize * (totalPages - 1))
			require.Len(t, getMessagesResp.Messages, remainingMessages)
		}

		allMessages = append(allMessages, getMessagesResp.Messages...)

		// Update maxTimestamp for the next iteration
		if getMessagesResp.NextMaxTimestamp != nil {
			maxTimestamp = *getMessagesResp.NextMaxTimestamp
		} else {
			break // No more messages to fetch
		}
	}

	// Verify that we've retrieved all 10 messages
	require.Len(t, allMessages, 10)

	// Verify that all messages are unique and in descending order of creation
	messageMap := make(map[string]bool)
	var lastTimestamp time.Time
	for _, message := range allMessages {
		require.False(t, messageMap[message.ID.String()], "Duplicate message found")
		messageMap[message.ID.String()] = true

		if !lastTimestamp.IsZero() {
			require.True(t, message.UpdatedAt.Before(lastTimestamp) || message.UpdatedAt.Equal(lastTimestamp), "Messages not in descending order")
		}
		lastTimestamp = message.UpdatedAt
	}
}

func testCreateMessageWithParent(t *testing.T, ts *TestServer, user *SignUpResponse, thread *storage.Thread, parentMessage *storage.CompoundMessage) {
	createMessageReq := CreateMessageRequest{
		ThreadID: thread.ID,
		ParentID: &storage.CompoundMessageID{
			ID:        parentMessage.ID,
			MessageID: 0,
		},
		Author:    user.AuthToken.UserID,
		Content:   "Reply to parent message",
		AuthToken: user.AuthToken,
	}
	createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/v0/create-message", createMessageReq)
	require.NoError(t, err)
	require.NotNil(t, createMessageResp.Message)
	require.Equal(t, parentMessage.ID, createMessageResp.Message.ParentID.ID)
	require.Equal(t, 0, createMessageResp.Message.ParentID.MessageID)

	// Verify the message is retrieved with the parent information
	getMessagesReq := GetMessagesRequest{
		ThreadID:     thread.ID,
		Limit:        1,
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/v0/get-messages", getMessagesReq)
	require.NoError(t, err)
	require.Len(t, getMessagesResp.Messages, 1)
	require.Equal(t, createMessageResp.Message.ID, getMessagesResp.Messages[0].ID)
	require.Equal(t, parentMessage.ID, getMessagesResp.Messages[0].ParentID.ID)
	require.Equal(t, 0, getMessagesResp.Messages[0].ParentID.MessageID)
}
