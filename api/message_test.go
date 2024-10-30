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

	// Create a space
	space := createTestSpace(t, ts, user)

	// Create a thread within the space
	thread := createTestThread(t, ts, user, space, "Test Thread")

	// Create 10 messages in the thread
	createdMessages := createTestMessages(t, ts, user, thread, 10)

	// Test message retrieval
	testMessageRetrieval(t, ts, user, thread, createdMessages)

	// Test pagination
	testMessagePagination(t, ts, user, thread)

	// Test creating a message with a parent
	testCreateMessageWithParent(t, ts, user, thread, createdMessages[0])
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
		createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", createMessageReq)
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
	getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/api/v0/get-messages", getMessagesReq)
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
		getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/api/v0/get-messages", getMessagesReq)
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
		require.Len(t, message.Messages, 1)
		require.Equal(t, "Test message", message.Messages[0])

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
	createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", createMessageReq)
	require.NoError(t, err)
	require.NotNil(t, createMessageResp.Message)
	require.Equal(t, parentMessage.ID, createMessageResp.Message.ParentID.ID)
	require.Equal(t, uint64(0), createMessageResp.Message.ParentID.MessageID)

	// Verify the message is retrieved with the parent information
	getMessagesReq := GetMessagesRequest{
		ThreadID:     thread.ID,
		Limit:        1,
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getMessagesResp, err := performRequest[GetMessagesRequest, GetMessagesResponse](t, ts, "POST", "/api/v0/get-messages", getMessagesReq)
	require.NoError(t, err)
	require.Len(t, getMessagesResp.Messages, 1)
	require.Equal(t, createMessageResp.Message.ID, getMessagesResp.Messages[0].ID)
	require.Equal(t, parentMessage.ID, getMessagesResp.Messages[0].ParentID.ID)
	require.Equal(t, uint64(0), getMessagesResp.Messages[0].ParentID.MessageID)
}

func TestMessageTimestampPropagation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	t.Run("Basic Timestamp Propagation", func(t *testing.T) {
		// Create test user and initial structure
		user, err := createTestUser(t, ts, "user1@example.com", "testuser1", "password123")
		require.NoError(t, err)

		// Create spaces and verify initial order
		space1 := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)
		space2 := createTestSpace(t, ts, user)

		getSpacesReq := GetSpacesRequest{
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Equal(t, space2.ID, getSpacesResp.Spaces[0].ID, "Initially, space2 should be first")

		// Create thread in space1
		thread := createTestThread(t, ts, user, space1, "Test Thread")
		time.Sleep(time.Millisecond * 10)

		// Create message and verify timestamp propagation
		createMessageReq := CreateMessageRequest{
			ThreadID:  thread.ID,
			Author:    "user",
			Content:   "Test message",
			AuthToken: user.AuthToken,
		}
		createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", createMessageReq)
		require.NoError(t, err)

		// Get updated spaces and verify order
		getSpacesResp, err = performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Equal(t, space1.ID, getSpacesResp.Spaces[0].ID, "After message creation, space1 should be first")
		require.Equal(t, createMessageResp.Message.UpdatedAt, getSpacesResp.Spaces[0].UpdatedAt)

		// Verify thread timestamp
		getThreadsReq := GetThreadsRequest{
			SpaceID:   space1.ID,
			Limit:     1,
			AuthToken: user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)
		require.Equal(t, createMessageResp.Message.UpdatedAt, getThreadsResp.Threads[0].UpdatedAt)
	})

	t.Run("Multiple Messages Timestamp Order", func(t *testing.T) {
		user, err := createTestUser(t, ts, "user2@example.com", "testuser2", "password123")
		require.NoError(t, err)

		space := createTestSpace(t, ts, user)
		thread := createTestThread(t, ts, user, space, "Test Thread")

		// Create multiple messages
		var lastTimestamp time.Time
		for i := 0; i < 3; i++ {
			createMessageReq := CreateMessageRequest{
				ThreadID:  thread.ID,
				Author:    "user",
				Content:   "Test message",
				AuthToken: user.AuthToken,
			}
			createMessageResp, err := performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", createMessageReq)
			require.NoError(t, err)

			// Verify thread timestamp is updated
			getThreadsReq := GetThreadsRequest{
				SpaceID:   space.ID,
				Limit:     1,
				AuthToken: user.AuthToken,
			}
			getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
			require.NoError(t, err)
			require.Equal(t, createMessageResp.Message.UpdatedAt, getThreadsResp.Threads[0].UpdatedAt)

			// Verify space timestamp is updated
			getSpacesReq := GetSpacesRequest{
				Limit:     1,
				AuthToken: user.AuthToken,
			}
			getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
			require.NoError(t, err)
			require.Equal(t, createMessageResp.Message.UpdatedAt, getSpacesResp.Spaces[0].UpdatedAt)

			if !lastTimestamp.IsZero() {
				require.True(t, createMessageResp.Message.UpdatedAt.After(lastTimestamp))
			}
			lastTimestamp = createMessageResp.Message.UpdatedAt
			time.Sleep(time.Millisecond * 10)
		}
	})

	t.Run("Pagination After Message Creation", func(t *testing.T) {
		user, err := createTestUser(t, ts, "user3@example.com", "testuser3", "password123")
		require.NoError(t, err)

		// Create multiple spaces
		spaces := createTestSpaces(t, ts, user, 5)
		middleSpace := spaces[2]

		// Create thread in middle space
		thread := createTestThread(t, ts, user, middleSpace, "Test Thread")

		// Create message to update timestamps
		createMessageReq := CreateMessageRequest{
			ThreadID:  thread.ID,
			Author:    "user",
			Content:   "Test message",
			AuthToken: user.AuthToken,
		}
		_, err = performRequest[CreateMessageRequest, CreateMessageResponse](t, ts, "POST", "/api/v0/create-message", createMessageReq)
		require.NoError(t, err)

		// Verify space pagination shows middle space first
		var allSpaces []*storage.Space
		var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())
		pageSize := 2

		for {
			getSpacesReq := GetSpacesRequest{
				Limit:        pageSize,
				MaxTimestamp: maxTimestamp,
				AuthToken:    user.AuthToken,
			}
			getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
			require.NoError(t, err)

			if len(getSpacesResp.Spaces) == 0 {
				break
			}

			allSpaces = append(allSpaces, getSpacesResp.Spaces...)

			if getSpacesResp.NextMaxTimestamp != nil {
				maxTimestamp = *getSpacesResp.NextMaxTimestamp
			} else {
				break
			}
		}

		require.Len(t, allSpaces, 5)
		require.Equal(t, middleSpace.ID, allSpaces[0].ID, "Space with new message should be first")
	})
}
