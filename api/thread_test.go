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

	// Create 15 threads for each user
	createTestThreads(t, ts, user1, 15)
	createTestThreads(t, ts, user2, 15)

	// Test thread retrieval for user1
	testThreadRetrieval(t, ts, user1, 15)

	// Test thread retrieval for user2
	testThreadRetrieval(t, ts, user2, 15)

	// Test pagination for user1
	testThreadPagination(t, ts, user1)

	// Test pagination for user2
	testThreadPagination(t, ts, user2)
}

func createTestThreads(t *testing.T, ts *TestServer, user *SignUpResponse, count int) {
	for i := 0; i < count; i++ {
		createThreadReq := CreateThreadRequest{
			Title:     fmt.Sprintf("Test Thread %d", i+1),
			AuthToken: user.AuthToken,
		}
		createThreadResp, err := performRequest[CreateThreadRequest, CreateThreadResponse](t, ts, "POST", "/v0/create-thread", createThreadReq)
		require.NoError(t, err)
		require.NotEmpty(t, createThreadResp.Thread.ID)
		require.NotEmpty(t, createThreadResp.Thread.UserID)
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
}

func testThreadRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, expectedCount int) {
	getThreadsReq := GetThreadsRequest{
		Limit:        expectedCount,
		MaxTimestamp: uint64(time.Now().UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/v0/get-threads", getThreadsReq)
	require.NoError(t, err)
	require.Len(t, getThreadsResp.Threads, expectedCount)

	// Verify that all threads belong to the user
	for _, thread := range getThreadsResp.Threads {
		require.Contains(t, thread.Title, "Test Thread")
	}
}

func testThreadPagination(t *testing.T, ts *TestServer, user *SignUpResponse) {
	pageSize := 5
	totalPages := 3

	var allThreads []*storage.Thread
	var maxTimestamp uint64 = uint64(time.Now().UnixNano())

	for page := 0; page < totalPages; page++ {
		getThreadsReq := GetThreadsRequest{
			Limit:        pageSize,
			MaxTimestamp: maxTimestamp,
			AuthToken:    user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/v0/get-threads", getThreadsReq)
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
