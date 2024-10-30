package api

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/nlpfollower/deltamind/database/db"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestThreadOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test users for basic operations
	user1, err := createTestUser(t, ts, "user1@example.com", "user1", "password1")
	require.NoError(t, err)
	user2, err := createTestUser(t, ts, "user2@example.com", "user2", "password2")
	require.NoError(t, err)

	t.Run("Single Thread Operations", func(t *testing.T) {
		// Create space for basic operations
		space := createTestSpace(t, ts, user1)

		// Test basic CRUD operations
		thread := createTestThread(t, ts, user1, space, "Test Thread")
		require.NotEmpty(t, thread.ID)
		require.Equal(t, space.ID, thread.SpaceID)
		require.Equal(t, "Test Thread", thread.Title)

		testThreadUpdate(t, ts, user1, user2, thread)
	})

	t.Run("Multiple Thread Operations", func(t *testing.T) {
		// Create fresh test server to ensure clean state
		ts := NewTestServer(t)
		defer ts.Close()

		// Create fresh users for this test
		user1, err := createTestUser(t, ts, "user1_multi@example.com", "user1_multi", "password1")
		require.NoError(t, err)
		user2, err := createTestUser(t, ts, "user2_multi@example.com", "user2_multi", "password2")
		require.NoError(t, err)

		// Create spaces for each user
		space1 := createTestSpace(t, ts, user1)
		space2 := createTestSpace(t, ts, user2)

		// Create threads for bulk operations
		threads1 := createTestThreads(t, ts, user1, space1, 15)
		threads2 := createTestThreads(t, ts, user2, space2, 15)

		testThreadRetrieval(t, ts, user1, space1, len(threads1))
		testThreadRetrieval(t, ts, user2, space2, len(threads2))
	})

	t.Run("Thread Pagination", func(t *testing.T) {
		// Create fresh test server to ensure clean state
		ts := NewTestServer(t)
		defer ts.Close()

		// Create fresh users for pagination test
		user1, err := createTestUser(t, ts, "user1_page@example.com", "user1_page", "password1")
		require.NoError(t, err)
		user2, err := createTestUser(t, ts, "user2_page@example.com", "user2_page", "password2")
		require.NoError(t, err)

		// Create fresh spaces for pagination test
		space1 := createTestSpace(t, ts, user1)
		space2 := createTestSpace(t, ts, user2)

		// Create threads specifically for pagination test
		createTestThreads(t, ts, user1, space1, 15)
		createTestThreads(t, ts, user2, space2, 15)

		testThreadPagination(t, ts, user1, space1)
		testThreadPagination(t, ts, user2, space2)
	})
}

func createTestThread(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space, title string) *storage.Thread {
	createThreadReq := CreateThreadRequest{
		SpaceID:   space.ID,
		Title:     title,
		AuthToken: user.AuthToken,
	}
	createThreadResp, err := performRequest[CreateThreadRequest, CreateThreadResponse](t, ts, "POST", "/api/v0/create-thread", createThreadReq)
	require.NoError(t, err)
	return &createThreadResp.Thread
}

func createTestThreads(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space, count int) []*storage.Thread {
	var threads []*storage.Thread
	for i := 0; i < count; i++ {
		thread := createTestThread(t, ts, user, space, fmt.Sprintf("Test Thread %d", i+1))
		threads = append(threads, thread)
		time.Sleep(time.Millisecond) // Ensure unique timestamps
	}
	return threads
}

func testThreadUpdate(t *testing.T, ts *TestServer, owner, otherUser *SignUpResponse, thread *storage.Thread) {
	// Test successful update
	updatedTitle := "Updated Test Thread"
	updateThreadReq := UpdateThreadRequest{
		ThreadID:  thread.ID,
		Title:     updatedTitle,
		AuthToken: owner.AuthToken,
	}
	updateThreadResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadReq)
	require.NoError(t, err)
	require.Equal(t, updatedTitle, updateThreadResp.Thread.Title)

	// Verify update through retrieval
	getThreadsReq := GetThreadsRequest{
		SpaceID:   thread.SpaceID,
		Limit:     1,
		AuthToken: owner.AuthToken,
	}
	getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
	require.NoError(t, err)
	require.Len(t, getThreadsResp.Threads, 1)
	require.Equal(t, updatedTitle, getThreadsResp.Threads[0].Title)

	// Test update with non-existent thread
	nonExistentThreadID, _ := db.DigestFromString("0123456789abcdef0123456789abcdef")
	updateNonExistentThreadReq := UpdateThreadRequest{
		ThreadID:  nonExistentThreadID,
		Title:     "Non-existent Thread",
		AuthToken: owner.AuthToken,
	}
	_, err = performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateNonExistentThreadReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")

	// Test update with invalid auth token
	invalidAuthToken := AuthToken{SessionKey: "invalid-session-key"}
	updateThreadInvalidAuthReq := UpdateThreadRequest{
		ThreadID:  thread.ID,
		Title:     "Updated with Invalid Auth",
		AuthToken: invalidAuthToken,
	}
	_, err = performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadInvalidAuthReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")

	// Test update without permission
	updateThreadNoPermReq := UpdateThreadRequest{
		ThreadID:  thread.ID,
		Title:     "Updated without Permission",
		AuthToken: otherUser.AuthToken,
	}
	_, err = performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadNoPermReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}

func testThreadRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, space *storage.Space, expectedCount int) {
	getThreadsReq := GetThreadsRequest{
		SpaceID:      space.ID,
		Limit:        expectedCount,
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
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
	var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())

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
	var lastTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())
	for _, thread := range allThreads {
		require.False(t, threadMap[thread.ID.String()], "Duplicate thread found")
		threadMap[thread.ID.String()] = true

		threadTimestamp := uint64(thread.UpdatedAt.UnixNano())
		require.LessOrEqual(t, threadTimestamp, lastTimestamp, "Threads not in descending order")
		lastTimestamp = threadTimestamp
	}
}

func TestThreadTimestampOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)
	space := createTestSpace(t, ts, user)

	t.Run("Update Timestamp Ordering", func(t *testing.T) {
		space := createTestSpace(t, ts, user) // Create fresh space for this test

		// Create initial thread
		thread := createTestThread(t, ts, user, space, "Initial Thread")
		time.Sleep(time.Millisecond * 10)

		// Create another thread to verify ordering
		thread2 := createTestThread(t, ts, user, space, "Second Thread")
		time.Sleep(time.Millisecond * 10)

		// Update first thread
		updateReq := UpdateThreadRequest{
			ThreadID:  thread.ID,
			Title:     "Updated Thread",
			AuthToken: user.AuthToken,
		}
		updateResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateReq)
		require.NoError(t, err)

		// Get threads and verify ordering
		getThreadsReq := GetThreadsRequest{
			SpaceID:   space.ID,
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)
		require.Len(t, getThreadsResp.Threads, 2)

		// Updated thread should be first due to newer timestamp
		require.Equal(t, updateResp.Thread.ID, getThreadsResp.Threads[0].ID)
		require.Equal(t, "Updated Thread", getThreadsResp.Threads[0].Title)
		require.Equal(t, thread2.ID, getThreadsResp.Threads[1].ID)

		// Verify timestamps are in correct order
		require.True(t, getThreadsResp.Threads[0].UpdatedAt.After(getThreadsResp.Threads[1].UpdatedAt))
	})

	t.Run("Multiple Updates", func(t *testing.T) {
		thread := createTestThread(t, ts, user, space, "Initial Thread")
		var lastUpdateTime time.Time

		// Perform multiple updates
		for i := 1; i <= 3; i++ {
			time.Sleep(time.Millisecond * 10) // Ensure distinct timestamps
			updateReq := UpdateThreadRequest{
				ThreadID:  thread.ID,
				Title:     fmt.Sprintf("Update %d", i),
				AuthToken: user.AuthToken,
			}
			updateResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateReq)
			require.NoError(t, err)

			// Verify timestamp is newer than last update
			if !lastUpdateTime.IsZero() {
				require.True(t, updateResp.Thread.UpdatedAt.After(lastUpdateTime))
			}
			lastUpdateTime = updateResp.Thread.UpdatedAt

			// Verify retrieval immediately after update
			getThreadsReq := GetThreadsRequest{
				SpaceID:   space.ID,
				Limit:     1,
				AuthToken: user.AuthToken,
			}
			getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
			require.NoError(t, err)
			require.Len(t, getThreadsResp.Threads, 1)
			require.Equal(t, fmt.Sprintf("Update %d", i), getThreadsResp.Threads[0].Title)
		}
	})

	t.Run("Concurrent Updates", func(t *testing.T) {
		thread := createTestThread(t, ts, user, space, "Initial Thread")
		var wg sync.WaitGroup
		updateCount := 5

		// Perform multiple updates concurrently
		for i := 1; i <= updateCount; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				updateReq := UpdateThreadRequest{
					ThreadID:  thread.ID,
					Title:     fmt.Sprintf("Concurrent Update %d", i),
					AuthToken: user.AuthToken,
				}
				_, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateReq)
				require.NoError(t, err)
			}(i)
		}
		wg.Wait()

		// Verify we can still retrieve the thread and it has a valid update
		getThreadsReq := GetThreadsRequest{
			SpaceID:   space.ID,
			Limit:     1,
			AuthToken: user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)
		require.Len(t, getThreadsResp.Threads, 1)
		require.Contains(t, getThreadsResp.Threads[0].Title, "Concurrent Update")
	})

	t.Run("Space Updates Should Not Affect Thread Ordering", func(t *testing.T) {
		// Create a thread
		thread := createTestThread(t, ts, user, space, "Thread Test")
		threadTime := thread.UpdatedAt
		time.Sleep(time.Millisecond * 10)

		// Update the space
		updateSpaceReq := UpdateSpaceRequest{
			SpaceID:     space.ID,
			Name:        "Updated Space",
			Description: "Updated Space Description",
			AuthToken:   user.AuthToken,
		}
		_, err := performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateSpaceReq)
		require.NoError(t, err)

		// Verify thread's timestamp wasn't affected
		getThreadsReq := GetThreadsRequest{
			SpaceID:   space.ID,
			Limit:     1,
			AuthToken: user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)
		require.Len(t, getThreadsResp.Threads, 1)
		require.Equal(t, threadTime, getThreadsResp.Threads[0].UpdatedAt)
	})

	t.Run("Pagination After Updates", func(t *testing.T) {
		space := createTestSpace(t, ts, user) // Create fresh space for this test

		// Create multiple threads
		threads := createTestThreads(t, ts, user, space, 5)
		time.Sleep(time.Millisecond * 10)

		// Update middle thread
		middleThread := threads[2]
		updateReq := UpdateThreadRequest{
			ThreadID:  middleThread.ID,
			Title:     "Updated Middle Thread",
			AuthToken: user.AuthToken,
		}
		updateResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateReq)
		require.NoError(t, err)

		// Get threads with pagination
		var allThreads []*storage.Thread
		var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())
		pageSize := 2

		for {
			getThreadsReq := GetThreadsRequest{
				SpaceID:      space.ID,
				Limit:        pageSize,
				MaxTimestamp: maxTimestamp,
				AuthToken:    user.AuthToken,
			}
			getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
			require.NoError(t, err)

			if len(getThreadsResp.Threads) == 0 {
				break
			}

			allThreads = append(allThreads, getThreadsResp.Threads...)

			if getThreadsResp.NextMaxTimestamp != nil {
				maxTimestamp = *getThreadsResp.NextMaxTimestamp
			} else {
				break
			}
		}

		// Verify updated thread is first and all threads are present
		require.Len(t, allThreads, 5)
		require.Equal(t, updateResp.Thread.ID, allThreads[0].ID)
		require.Equal(t, "Updated Middle Thread", allThreads[0].Title)
	})

	t.Run("Delete and Recreate", func(t *testing.T) {
		space := createTestSpace(t, ts, user) // Create fresh space for this test

		// Create initial thread
		thread := createTestThread(t, ts, user, space, "Initial Thread")
		originalID := thread.ID
		time.Sleep(time.Millisecond * 10)

		// Delete the thread
		deleteReq := DeleteThreadRequest{
			ThreadID:  thread.ID,
			AuthToken: user.AuthToken,
		}
		deleteResp, err := performRequest[DeleteThreadRequest, DeleteThreadResponse](t, ts, "POST", "/api/v0/delete-thread", deleteReq)
		require.NoError(t, err)
		require.True(t, deleteResp.Success)

		// Create new thread with same title
		newThread := createTestThread(t, ts, user, space, "Initial Thread")
		require.NotEqual(t, originalID, newThread.ID, "New thread should have different ID")

		// Verify only new thread exists
		getThreadsReq := GetThreadsRequest{
			SpaceID:   space.ID,
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/api/v0/get-threads", getThreadsReq)
		require.NoError(t, err)

		found := false
		for _, t := range getThreadsResp.Threads {
			if t.ID == originalID {
				found = true
				break
			}
		}
		require.False(t, found, "Original thread should not exist")
	})

	t.Run("Update Chain", func(t *testing.T) {
		// Create initial thread
		thread := createTestThread(t, ts, user, space, "Initial Thread")
		var timestamps []time.Time

		// Perform chain of updates without delay
		for i := 1; i <= 3; i++ {
			updateReq := UpdateThreadRequest{
				ThreadID:  thread.ID,
				Title:     fmt.Sprintf("Update Chain %d", i),
				AuthToken: user.AuthToken,
			}
			updateResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateReq)
			require.NoError(t, err)
			timestamps = append(timestamps, updateResp.Thread.UpdatedAt)
		}

		// Verify all timestamps are different even without sleep
		for i := 1; i < len(timestamps); i++ {
			require.True(t, timestamps[i].After(timestamps[i-1]),
				"Each update should have a newer timestamp even without delay")
		}
	})
}

func TestSpaceThreadTimestampPropagation(t *testing.T) {
	t.Run("Space Timestamp Updates After Thread Operations", func(t *testing.T) {
		// Fresh test server for this subtest
		ts := NewTestServer(t)
		defer ts.Close()
		user, err := createTestUser(t, ts, "user1@example.com", "testuser1", "password123")
		require.NoError(t, err)

		// Create two spaces with a small delay to ensure ordered timestamps
		space1 := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)
		space2 := createTestSpace(t, ts, user)

		// Initially, space2 should be first due to being newer
		getSpacesReq := GetSpacesRequest{
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 2)
		require.Equal(t, space2.ID, getSpacesResp.Spaces[0].ID)

		// Create and then update a thread in space1
		thread := createTestThread(t, ts, user, space1, "Test Thread")
		time.Sleep(time.Millisecond * 10)

		updateThreadReq := UpdateThreadRequest{
			ThreadID:  thread.ID,
			Title:     "Updated Thread",
			AuthToken: user.AuthToken,
		}
		updateThreadResp, err := performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadReq)
		require.NoError(t, err)

		// Now space1 should be first due to thread update
		getSpacesResp, err = performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 2)
		require.Equal(t, space1.ID, getSpacesResp.Spaces[0].ID)

		// Verify space1's timestamp matches the thread update time
		require.Equal(t, updateThreadResp.Thread.UpdatedAt, getSpacesResp.Spaces[0].UpdatedAt)
	})

	t.Run("Space Pagination After Thread Updates", func(t *testing.T) {
		// Fresh test server for this subtest
		ts := NewTestServer(t)
		defer ts.Close()
		user, err := createTestUser(t, ts, "user2@example.com", "testuser2", "password123")
		require.NoError(t, err)

		// Create multiple spaces
		spaces := createTestSpaces(t, ts, user, 5)
		middleSpace := spaces[2]

		// Create and update a thread in the middle space
		thread := createTestThread(t, ts, user, middleSpace, "Test Thread")
		time.Sleep(time.Millisecond * 10)

		updateThreadReq := UpdateThreadRequest{
			ThreadID:  thread.ID,
			Title:     "Updated Thread",
			AuthToken: user.AuthToken,
		}
		_, err = performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadReq)
		require.NoError(t, err)

		// Verify space pagination shows updated space first
		pageSize := 2
		var allSpaces []*storage.Space
		var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())

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
		require.Equal(t, middleSpace.ID, allSpaces[0].ID, "Space with updated thread should be first")
	})

	t.Run("Thread Operations Maintain Space Order", func(t *testing.T) {
		// Fresh test server for this subtest
		ts := NewTestServer(t)
		defer ts.Close()
		user, err := createTestUser(t, ts, "user3@example.com", "testuser3", "password123")
		require.NoError(t, err)

		space1 := createTestSpace(t, ts, user)
		space2 := createTestSpace(t, ts, user)

		// Create threads in both spaces
		thread1 := createTestThread(t, ts, user, space1, "Thread 1")
		_ = createTestThread(t, ts, user, space2, "Thread 2")

		// Update thread1 after thread2 is created
		updateThreadReq := UpdateThreadRequest{
			ThreadID:  thread1.ID,
			Title:     "Updated Thread 1",
			AuthToken: user.AuthToken,
		}
		_, err = performRequest[UpdateThreadRequest, UpdateThreadResponse](t, ts, "POST", "/api/v0/update-thread", updateThreadReq)
		require.NoError(t, err)

		// Verify space1 is now first in listing due to thread update
		getSpacesReq := GetSpacesRequest{
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 2, "Should have exactly 2 spaces")

		// Spaces should be in the correct order
		require.Equal(t, space1.ID, getSpacesResp.Spaces[0].ID, "Space1 should be first")
		require.Equal(t, space2.ID, getSpacesResp.Spaces[1].ID, "Space2 should be second")
	})

	t.Run("Space Timestamp Updates on Thread Deletion", func(t *testing.T) {
		// Fresh test server for this subtest
		ts := NewTestServer(t)
		defer ts.Close()
		user, err := createTestUser(t, ts, "user4@example.com", "testuser4", "password123")
		require.NoError(t, err)

		space1 := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)
		space2 := createTestSpace(t, ts, user)

		// Create and then delete a thread in space1
		thread := createTestThread(t, ts, user, space1, "Test Thread")
		time.Sleep(time.Millisecond * 10)

		deleteThreadReq := DeleteThreadRequest{
			ThreadID:  thread.ID,
			AuthToken: user.AuthToken,
		}
		_, err = performRequest[DeleteThreadRequest, DeleteThreadResponse](t, ts, "POST", "/api/v0/delete-thread", deleteThreadReq)
		require.NoError(t, err)

		// Verify space1 timestamp was updated and is now newer than space2
		getSpacesReq := GetSpacesRequest{
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 2, "Should have exactly 2 spaces")

		require.Equal(t, space1.ID, getSpacesResp.Spaces[0].ID, "Space1 should be first")
		require.Equal(t, space2.ID, getSpacesResp.Spaces[1].ID, "Space2 should be second")
		require.True(t, getSpacesResp.Spaces[0].UpdatedAt.After(getSpacesResp.Spaces[1].UpdatedAt),
			"Space1 should have newer timestamp after thread deletion")
	})
}
