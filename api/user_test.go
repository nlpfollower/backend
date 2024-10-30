package api

import (
	"encoding/hex"
	"github.com/nlpfollower/deltamind/backend/storage"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignUpAndSignIn(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Test SignUp
	signUpResp, err := createTestUser(t, ts, "test@example.com", "testuser", "userPassword123")
	require.NoError(t, err)
	require.NotEmpty(t, signUpResp.AuthToken.SessionKey)

	// Test SignIn
	signInReq := SignInRequest{
		Email:       "test@example.com",
		PasswordHex: hex.EncodeToString(HashPassword("userPassword123")),
	}
	signInResp, err := performRequest[SignInRequest, SignInResponse](t, ts, "POST", "/api/v0/sign-in", signInReq)
	require.NoError(t, err)
	require.NotEmpty(t, signInResp.AuthToken.SessionKey)

	// Test double signup
	_, err = createTestUser(t, ts, "test@example.com", "testuser", "userPassword123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status code: 409")

	// Test SignIn with modified password
	modifiedPasswordHex := modifyHexString(hex.EncodeToString(HashPassword("userPassword123")))
	modifiedSignInReq := SignInRequest{
		Email:       "test@example.com",
		PasswordHex: modifiedPasswordHex,
	}
	_, err = performRequest[SignInRequest, SignInResponse](t, ts, "POST", "/api/v0/sign-in", modifiedSignInReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status code: 401")

	// Test SetSpace with valid AuthToken to verify AuthToken functionality
	createSpaceReq := CreateSpaceRequest{
		Name:        "Test Space",
		Description: "A test space",
		AuthToken:   signUpResp.AuthToken,
	}
	createSpaceResp, err := performRequest[CreateSpaceRequest, CreateSpaceResponse](t, ts, "POST", "/api/v0/create-space", createSpaceReq)
	require.NoError(t, err)
	require.NotEmpty(t, createSpaceResp.Space.ID)
	require.Equal(t, "Test Space", createSpaceResp.Space.Name)

	// Verify the space exists by listing spaces
	getSpacesReq := GetSpacesRequest{
		Limit:     10,
		AuthToken: signUpResp.AuthToken,
	}
	getSpacesResp, err := performRequest[GetSpacesRequest, GetSpacesResponse](t, ts, "POST", "/api/v0/get-spaces", getSpacesReq)
	require.NoError(t, err)
	require.NotEmpty(t, getSpacesResp.Spaces)

	foundNewSpace := false
	for _, space := range getSpacesResp.Spaces {
		if space.ID == createSpaceResp.Space.ID {
			foundNewSpace = true
			require.Equal(t, "Test Space", space.Name)
			break
		}
	}
	require.True(t, foundNewSpace, "Newly created space not found in the list of spaces")
}

func createTestUser(t *testing.T, ts *TestServer, email, username, password string) (*SignUpResponse, error) {
	processedPassword := HashPassword(password)
	hexEncodedPassword := hex.EncodeToString(processedPassword)

	signUpReq := SignUpRequest{
		Email:       email,
		Username:    username,
		PasswordHex: hexEncodedPassword,
		Method:      storage.AuthMethodEmailPassword,
	}

	return performRequest[SignUpRequest, SignUpResponse](t, ts, "POST", "/api/v0/sign-up", signUpReq)
}

func modifyHexString(hexStr string) string {
	bytes, _ := hex.DecodeString(hexStr)
	if len(bytes) > 0 {
		bytes[0] ^= 0x01 // Flip the least significant bit of the first byte
	}
	return hex.EncodeToString(bytes)
}
