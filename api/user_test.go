package api

import (
	"encoding/hex"
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
	signInResp, err := performRequest[SignInRequest, SignInResponse](t, ts, "POST", "/v0/sign-in", signInReq)
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
	_, err = performRequest[SignInRequest, SignInResponse](t, ts, "POST", "/v0/sign-in", modifiedSignInReq)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status code: 401")

	// Test CreateThread with valid AuthToken
	createThreadReq := CreateThreadRequest{
		Title:     "Test Thread",
		AuthToken: signUpResp.AuthToken,
	}
	createThreadResp, err := performRequest[CreateThreadRequest, CreateThreadResponse](t, ts, "POST", "/v0/create-thread", createThreadReq)
	require.NoError(t, err)
	require.NotEmpty(t, createThreadResp.Thread.ID)
	require.NotEmpty(t, createThreadResp.Thread.UserID)

	// List threads and check if the newly created thread exists
	getThreadsReq := GetThreadsRequest{
		Limit:     10,
		AuthToken: signUpResp.AuthToken,
	}
	getThreadsResp, err := performRequest[GetThreadsRequest, GetThreadsResponse](t, ts, "POST", "/v0/get-threads", getThreadsReq)
	require.NoError(t, err)
	require.NotEmpty(t, getThreadsResp.Threads)

	foundNewThread := false
	for _, thread := range getThreadsResp.Threads {
		if thread.ID == createThreadResp.Thread.ID {
			foundNewThread = true
			require.Equal(t, "Test Thread", thread.Title)
			break
		}
	}
	require.True(t, foundNewThread, "Newly created thread not found in the list of threads")
}

func createTestUser(t *testing.T, ts *TestServer, email, username, password string) (*SignUpResponse, error) {
	processedPassword := HashPassword(password)
	hexEncodedPassword := hex.EncodeToString(processedPassword)

	signUpReq := SignUpRequest{
		Email:       email,
		Username:    username,
		PasswordHex: hexEncodedPassword,
		Method:      AuthMethodEmailPassword,
	}

	return performRequest[SignUpRequest, SignUpResponse](t, ts, "POST", "/v0/sign-up", signUpReq)
}

func modifyHexString(hexStr string) string {
	bytes, _ := hex.DecodeString(hexStr)
	if len(bytes) > 0 {
		bytes[0] ^= 0x01 // Flip the least significant bit of the first byte
	}
	return hex.EncodeToString(bytes)
}
