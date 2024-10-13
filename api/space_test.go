package api

import (
	"fmt"
	"github.com/nlpfollower/deltamind/backend/storage"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSpaceCreationAndRetrieval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user
	user, err := createTestUser(t, ts, "user@example.com", "testuser", "password123")
	require.NoError(t, err)

	// Create 5 spaces
	createdSpaces := createTestSpaces(t, ts, user, 5)

	// Test space retrieval
	testSpaceRetrieval(t, ts, user, createdSpaces)

	// Test pagination
	testSpacePagination(t, ts, user)

	// Test space deletion
	testSpaceDeletion(t, ts, user, createdSpaces[0])
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
		// Add a small delay to ensure unique timestamps
		time.Sleep(time.Millisecond)
	}
	return spaces
}

func testSpaceRetrieval(t *testing.T, ts *TestServer, user *SignUpResponse, createdSpaces []*storage.Space) {
	getSpacesReq := GetSpacesRequest{
		Limit:        len(createdSpaces),
		MaxTimestamp: uint64(time.Now().Add(time.Hour).UnixNano()), // Use a future timestamp to get all spaces
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
			// On the last page, we expect the remaining spaces
			remainingSpaces := 5 - (pageSize * (totalPages - 1))
			require.Len(t, getSpacesResp.Spaces, remainingSpaces)
		}

		allSpaces = append(allSpaces, getSpacesResp.Spaces...)

		// Update maxTimestamp for the next iteration
		if getSpacesResp.NextMaxTimestamp != nil {
			maxTimestamp = *getSpacesResp.NextMaxTimestamp
		} else {
			break // No more spaces to fetch
		}
	}

	// Verify that we've retrieved all 5 spaces
	require.Len(t, allSpaces, 5)

	// Verify that all spaces are unique and in descending order of creation
	spaceMap := make(map[string]bool)
	var lastTimestamp time.Time
	for _, space := range allSpaces {
		require.False(t, spaceMap[space.ID.String()], "Duplicate space found")
		spaceMap[space.ID.String()] = true

		if !lastTimestamp.IsZero() {
			require.True(t, space.UpdatedAt.Before(lastTimestamp) || space.UpdatedAt.Equal(lastTimestamp), "Spaces not in descending order")
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

	// Verify the space was deleted
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
