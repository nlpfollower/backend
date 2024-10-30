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

func TestSpaceOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test users that we'll use across all tests
	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)
	otherUser, err := createTestUser(t, ts, "other@example.com", "otheruser", "password123")
	require.NoError(t, err)

	t.Run("Single Space Operations", func(t *testing.T) {
		// Test basic CRUD operations on a single space
		space := createTestSpace(t, ts, user)
		require.NotEmpty(t, space.ID)
		require.Equal(t, "Test Space", space.Name)
		require.Equal(t, "A test space for threads", space.Description)

		testSpaceUpdate(t, ts, user, otherUser, space)
		testSpaceDeletion(t, ts, user, space)
	})

	t.Run("Multiple Space Operations", func(t *testing.T) {
		// Create fresh test server to ensure clean state
		ts := NewTestServer(t)
		defer ts.Close()

		// Create fresh user for this test
		user, err := createTestUser(t, ts, "user_multi@example.com", "testuser_multi", "password123")
		require.NoError(t, err)

		// Create spaces for bulk operations
		spaces := createTestSpaces(t, ts, user, 5)
		testSpaceRetrieval(t, ts, user, spaces)
	})

	t.Run("Space Pagination", func(t *testing.T) {
		// Create fresh test server to ensure clean state
		ts := NewTestServer(t)
		defer ts.Close()

		// Create fresh user for this test
		user, err := createTestUser(t, ts, "user_page@example.com", "testuser_page", "password123")
		require.NoError(t, err)

		// Create spaces specifically for pagination test
		createTestSpaces(t, ts, user, 5)
		testSpacePagination(t, ts, user)
	})
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

func createTestSpaces(t *testing.T, ts *TestServer, user *SignUpResponse, count int) []*storage.Space {
	var spaces []*storage.Space
	for i := 0; i < count; i++ {
		createSpaceReq := CreateSpaceRequest{
			Name:        fmt.Sprintf("Test Space %d", i+1),
			Description: fmt.Sprintf("Description for Test Space %d", i+1),
			AuthToken:   user.AuthToken,
		}
		createSpaceResp, err := performRequest[CreateSpaceRequest, CreateSpaceResponse](t, ts, "POST", "/api/v0/create-space", createSpaceReq)
		require.NoError(t, err)
		require.NotNil(t, createSpaceResp.Space)
		spaces = append(spaces, &createSpaceResp.Space)
		time.Sleep(time.Millisecond) // Ensure unique timestamps
	}
	return spaces
}

func testSpaceUpdate(t *testing.T, ts *TestServer, owner, otherUser *SignUpResponse, space *storage.Space) {
	// Test successful update
	updatedName := "Updated Test Space"
	updatedDescription := "Updated test space description"
	updateSpaceReq := UpdateSpaceRequest{
		SpaceID:     space.ID,
		Name:        updatedName,
		Description: updatedDescription,
		AuthToken:   owner.AuthToken,
	}
	updateSpaceResp, err := performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateSpaceReq)
	require.NoError(t, err)
	require.Equal(t, updatedName, updateSpaceResp.Space.Name)
	require.Equal(t, updatedDescription, updateSpaceResp.Space.Description)

	// Verify update through retrieval
	getSpacesReq := GetSpacesRequest{
		Limit:     1,
		AuthToken: owner.AuthToken,
	}
	getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
	require.NoError(t, err)
	require.Len(t, getSpacesResp.Spaces, 1)
	require.Equal(t, updatedName, getSpacesResp.Spaces[0].Name)
	require.Equal(t, updatedDescription, getSpacesResp.Spaces[0].Description)

	// Test update with non-existent space
	nonExistentSpaceID, _ := db.DigestFromString("0123456789abcdef0123456789abcdef")
	updateNonExistentSpaceReq := UpdateSpaceRequest{
		SpaceID:     nonExistentSpaceID,
		Name:        "Non-existent Space",
		Description: "This space doesn't exist",
		AuthToken:   owner.AuthToken,
	}
	_, err = performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateNonExistentSpaceReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")

	// Test update with invalid auth token
	invalidAuthToken := AuthToken{SessionKey: "invalid-session-key"}
	updateSpaceInvalidAuthReq := UpdateSpaceRequest{
		SpaceID:     space.ID,
		Name:        "Updated with Invalid Auth",
		Description: "This update should fail",
		AuthToken:   invalidAuthToken,
	}
	_, err = performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateSpaceInvalidAuthReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")

	// Test update without permission using otherUser
	updateSpaceNoPermReq := UpdateSpaceRequest{
		SpaceID:     space.ID,
		Name:        "Updated without Permission",
		Description: "This update should fail",
		AuthToken:   otherUser.AuthToken,
	}
	_, err = performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateSpaceNoPermReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}

func testSpaceRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, createdSpaces []*storage.Space) {
	getSpacesReq := GetSpacesRequest{
		Limit:        len(createdSpaces),
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
	require.NoError(t, err)
	require.Len(t, getSpacesResp.Spaces, len(createdSpaces))

	// Verify that all spaces are retrieved in reverse order
	for i, space := range getSpacesResp.Spaces {
		require.Equal(t, createdSpaces[len(createdSpaces)-1-i].ID, space.ID)
		require.Equal(t, createdSpaces[len(createdSpaces)-1-i].Name, space.Name)
		require.Equal(t, createdSpaces[len(createdSpaces)-1-i].Description, space.Description)
	}

	// Verify NextMaxTimestamp
	require.NotNil(t, getSpacesResp.NextMaxTimestamp)
	require.Equal(t, uint64(createdSpaces[0].UpdatedAt.UnixNano()), *getSpacesResp.NextMaxTimestamp)
}

func testSpacePagination(t *testing.T, ts *TestServer, user *SignUpResponse) {
	pageSize := 2
	totalPages := 3

	var allSpaces []*storage.Space
	var maxTimestamp uint64 = uint64(time.Now().Add(time.Hour).UnixNano())

	for page := 0; page < totalPages; page++ {
		getSpacesReq := GetSpacesRequest{
			Limit:        pageSize,
			MaxTimestamp: maxTimestamp,
			AuthToken:    user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)

		if page < totalPages-1 {
			require.Len(t, getSpacesResp.Spaces, pageSize)
		} else {
			remainingSpaces := 5 - (pageSize * (totalPages - 1))
			require.Len(t, getSpacesResp.Spaces, remainingSpaces)
		}

		allSpaces = append(allSpaces, getSpacesResp.Spaces...)

		if getSpacesResp.NextMaxTimestamp != nil {
			maxTimestamp = *getSpacesResp.NextMaxTimestamp
		} else {
			break
		}
	}

	// Verify that we've retrieved all 5 spaces
	require.Len(t, allSpaces, 5)

	// Verify uniqueness and ordering
	spaceMap := make(map[string]bool)
	var lastTimestamp time.Time
	for _, space := range allSpaces {
		require.False(t, spaceMap[space.ID.String()], "Duplicate space found")
		spaceMap[space.ID.String()] = true

		if !lastTimestamp.IsZero() {
			require.True(t, space.UpdatedAt.Before(lastTimestamp) || space.UpdatedAt.Equal(lastTimestamp),
				"Spaces not in descending order")
		}
		lastTimestamp = space.UpdatedAt
	}
}

func testSpaceDeletion(t *testing.T, ts *TestServer, user *SignUpResponse, spaceToDelete *storage.Space) {
	deleteSpaceReq := DeleteSpaceRequest{
		SpaceID:   spaceToDelete.ID,
		AuthToken: user.AuthToken,
	}
	deleteSpaceResp, err := performRequest[DeleteSpaceRequest, DeleteSpaceResponse](t, ts, "POST", "/api/v0/delete-space", deleteSpaceReq)
	require.NoError(t, err)
	require.True(t, deleteSpaceResp.Success)

	// Verify deletion
	getSpacesReq := GetSpacesRequest{
		Limit:        10,
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()),
		AuthToken:    user.AuthToken,
	}
	getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
	require.NoError(t, err)

	for _, space := range getSpacesResp.Spaces {
		require.NotEqual(t, spaceToDelete.ID, space.ID, "Deleted space should not be retrieved")
	}
}

func TestSpaceTimestampOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)

	t.Run("Update Timestamp Ordering", func(t *testing.T) {
		// Create initial space
		space := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)

		// Create another space to verify ordering
		space2 := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)

		// Update first space
		updateReq := UpdateSpaceRequest{
			SpaceID:     space.ID,
			Name:        "Updated Space",
			Description: "Updated Description",
			AuthToken:   user.AuthToken,
		}
		updateResp, err := performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateReq)
		require.NoError(t, err)

		// Get spaces and verify ordering
		getSpacesReq := GetSpacesRequest{
			Limit:     10,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 2)

		// Updated space should be first due to newer timestamp
		require.Equal(t, updateResp.Space.ID, getSpacesResp.Spaces[0].ID)
		require.Equal(t, "Updated Space", getSpacesResp.Spaces[0].Name)
		require.Equal(t, space2.ID, getSpacesResp.Spaces[1].ID)

		// Verify timestamps are in correct order
		require.True(t, getSpacesResp.Spaces[0].UpdatedAt.After(getSpacesResp.Spaces[1].UpdatedAt))
	})

	t.Run("Multiple Updates", func(t *testing.T) {
		space := createTestSpace(t, ts, user)
		var lastUpdateTime time.Time

		// Perform multiple updates
		for i := 1; i <= 3; i++ {
			time.Sleep(time.Millisecond * 10) // Ensure distinct timestamps
			updateReq := UpdateSpaceRequest{
				SpaceID:     space.ID,
				Name:        fmt.Sprintf("Update %d", i),
				Description: fmt.Sprintf("Description %d", i),
				AuthToken:   user.AuthToken,
			}
			updateResp, err := performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateReq)
			require.NoError(t, err)

			// Verify timestamp is newer than last update
			if !lastUpdateTime.IsZero() {
				require.True(t, updateResp.Space.UpdatedAt.After(lastUpdateTime))
			}
			lastUpdateTime = updateResp.Space.UpdatedAt

			// Verify retrieval immediately after update
			getSpacesReq := GetSpacesRequest{
				Limit:     1,
				AuthToken: user.AuthToken,
			}
			getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
			require.NoError(t, err)
			require.Len(t, getSpacesResp.Spaces, 1)
			require.Equal(t, fmt.Sprintf("Update %d", i), getSpacesResp.Spaces[0].Name)
		}
	})

	t.Run("Update After Delete", func(t *testing.T) {
		// Create space
		space := createTestSpace(t, ts, user)
		time.Sleep(time.Millisecond * 10)

		// Delete space
		deleteReq := DeleteSpaceRequest{
			SpaceID:   space.ID,
			AuthToken: user.AuthToken,
		}
		_, err := performRequest[DeleteSpaceRequest, DeleteSpaceResponse](t, ts, "POST", "/api/v0/delete-space", deleteReq)
		require.NoError(t, err)

		// Attempt update after delete
		updateReq := UpdateSpaceRequest{
			SpaceID:     space.ID,
			Name:        "Updated After Delete",
			Description: "This should fail",
			AuthToken:   user.AuthToken,
		}
		_, err = performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})

	t.Run("Concurrent Updates", func(t *testing.T) {
		space := createTestSpace(t, ts, user)
		var wg sync.WaitGroup
		updateCount := 5

		// Perform multiple updates concurrently
		for i := 1; i <= updateCount; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				updateReq := UpdateSpaceRequest{
					SpaceID:     space.ID,
					Name:        fmt.Sprintf("Concurrent Update %d", i),
					Description: fmt.Sprintf("Concurrent Description %d", i),
					AuthToken:   user.AuthToken,
				}
				_, err := performRequest[UpdateSpaceRequest, UpdateSpaceResponse](t, ts, "POST", "/api/v0/update-space", updateReq)
				//fmt.Println(resp)
				require.NoError(t, err)
			}(i)
		}
		wg.Wait()

		// Verify we can still retrieve the space and it has a valid update
		getSpacesReq := GetSpacesRequest{
			Limit:     1,
			AuthToken: user.AuthToken,
		}
		getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
		require.NoError(t, err)
		require.Len(t, getSpacesResp.Spaces, 1)
		require.Contains(t, getSpacesResp.Spaces[0].Name, "Concurrent Update")
	})
}
