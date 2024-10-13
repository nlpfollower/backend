package api

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestThreadCreationAndRetrieval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create two users
	user1, err := createTestUser(t, ts, "user1@example.com", "user1", "password1")
	require.NoError(t, err)
	user2, err := createTestUser(t, ts, "user2@example.com", "user2", "password2")
	require.NoError(t, err)

	// Create a space for each user
	space1 := createTestSpace(t, ts, user1)
	space2 := createTestSpace(t, ts, user2)

	// Create 15 threads for each user's space
	createTestThreads(t, ts, user1, space1, 15)
	createTestThreads(t, ts, user2, space2, 15)

	// Test thread retrieval for user1's space
	testThreadRetrieval(t, ts, user1, space1, 15)

	// Test thread retrieval for user2's space
	testThreadRetrieval(t, ts, user2, space2, 15)

	// Test pagination for user1's space
	testThreadPagination(t, ts, user1, space1)

	// Test pagination for user2's space
	testThreadPagination(t, ts, user2, space2)
}

func createTestSpace(t *testing.T, ts *TestServer, user *SignUpResponse) *storage.Space {
	createSpaceReq := CreateSpaceRequest{
		Name:        "Test Space",
		Description: "A test space for threads",
		AuthToken:   user.AuthToken,
	}
	createSpaceResp, err := performRequest[CreateSpaceRequest, CreateSpaceResponse](t, ts, "POST", "/api/v0/create-space", createSpaceReq)
	require.NoError(t, err)
	require.NotEmpty(t, createSpaceResp.Space.ID)
	return &createSpaceResp.Space
}

func createTestThreads(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space, count int) {
	for i := 0; i < count; i++ {
		createThreadReq := CreateThreadRequest{
			SpaceID:   space.ID,
			Title:     fmt.Sprintf("Test Thread %d", i+1),
			AuthToken: user.AuthToken,
		}
		createThreadResp, err := performRequest[CreateThreadRequest, CreateThreadResponse](t, ts, "POST", "/api/v0/create-thread", createThreadReq)
		require.NoError(t, err)
		require.NotEmpty(t, createThreadResp.Thread.ID)
		require.Equal(t, space.ID, createThreadResp.Thread.SpaceID)
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
}

func testThreadRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space, expectedCount int) {
	getThreadsReq := GetThreadsRequest{
		SpaceID:      space.ID,
		Limit:        expectedCount,
		MaxTimestamp: uint64(time.Now().UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
	require.NoError(t, err)
	require.Len(t, getThreadsResp.Threads, expectedCount)

	// Verify that all threads belong to the space
	for _, thread := range getThreadsResp.Threads {
		require.Equal(t, space.ID, thread.SpaceID)
		require.Contains(t, thread.Title, "Test Thread")
	}
}

func testThreadPagination(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space) {
	pageSize := 5
	totalPages := 3

	var allThreads []*storage.Thread
	var maxTimestamp uint64 = uint64(time.Now().UnixNano())

	for page := 0; page < totalPages; page++ {
		getThreadsReq := GetThreadsRequest{
			SpaceID:      space.ID,
			Limit:        pageSize,
			MaxTimestamp: maxTimestamp,
			AuthToken:    user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)

		if page < totalPages-1 {
			require.Len(t, getThreadsResp.Threads, pageSize)
		} else {
			// On the last page, we expect the remaining threads
			remainingThreads := 15 - (pageSize * (totalPages - 1))
			require.Len(t, getThreadsResp.Threads, remainingThreads)
		}

		allThreads = append(allThreads, getThreadsResp.Threads...)

		// Update maxTimestamp for the next iteration
		if getThreadsResp.NextMaxTimestamp != nil {
			maxTimestamp = *getThreadsResp.NextMaxTimestamp
		} else {
			break // No more threads to fetch
		}
	}

	// Verify that we've retrieved all 15 threads
	require.Len(t, allThreads, 15)

	// Verify that all threads are unique and in descending order of creation
	threadMap := make(map[string]bool)
	var lastTimestamp uint64 = uint64(time.Now().UnixNano())
	for _, thread := range allThreads {
		require.False(t, threadMap[thread.ID.String()], "Duplicate thread found")
		threadMap[thread.ID.String()] = true

		threadTimestamp := uint64(thread.UpdatedAt.UnixNano())
		require.LessOrEqual(t, threadTimestamp, lastTimestamp, "Threads not in descending order")
		lastTimestamp = threadTimestamp
	}
}
