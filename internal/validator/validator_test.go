package validator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"telnyx-mock/internal/database"
)

func setupTestDB(t *testing.T) func() {
	testDBPath := "test_validator.db"
	err := database.InitDB(testDBPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return func() {
		database.CloseDB()
		os.Remove(testDBPath)
	}
}

func TestValidateMessageRequest_MissingFrom(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		To:                 "+1234567890",
		Text:               "Hello",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
	if errResp != nil && errResp.Errors[0].Code != "10005" {
		t.Errorf("Expected error code 10005, got %s", errResp.Errors[0].Code)
	}
}

func TestValidateMessageRequest_MissingTo(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		Text:               "Hello",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
}

func TestValidateMessageRequest_MissingMessagingProfileID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From: "+1234567890",
		To:   "+0987654321",
		Text: "Hello",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
	if errResp != nil && errResp.Errors[0].Detail != "The 'messaging_profile_id' parameter is required." {
		t.Errorf("Unexpected error detail: %s", errResp.Errors[0].Detail)
	}
}

func TestValidateMessageRequest_MissingTextAndMediaURLs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
}

func TestValidateMessageRequest_MissingAuthHeader(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	// No Authorization header

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Hello",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
	if errResp != nil && errResp.Errors[0].Code != "10001" {
		t.Errorf("Expected error code 10001, got %s", errResp.Errors[0].Code)
	}
}

func TestValidateMessageRequest_InvalidAuthToken(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Hello",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, statusCode)
	}
	if errResp == nil {
		t.Error("Expected error response, got nil")
	}
}

func TestValidateMessageRequest_ValidRequest(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Hello, World!",
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	if statusCode != 0 {
		t.Errorf("Expected status 0 (valid), got %d", statusCode)
	}
	if errResp != nil {
		t.Errorf("Expected no error response, got %+v", errResp)
	}
}

func TestValidateMessageRequest_WithMediaURLsNoText(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		MediaURLs:          []string{"https://example.com/image.jpg"},
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	// Should be valid - media_urls is acceptable without text
	if statusCode != 0 {
		t.Errorf("Expected status 0 (valid), got %d", statusCode)
	}
	if errResp != nil {
		t.Errorf("Expected no error response, got %+v", errResp)
	}
}

func TestValidateMessageRequest_WithBothTextAndMediaURLs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	msgReq := &MessageRequest{
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Check out this image!",
		MediaURLs:          []string{"https://example.com/image.jpg"},
		MessagingProfileID: "profile-123",
	}

	statusCode, errResp := ValidateMessageRequest(req, msgReq)

	// Should be valid
	if statusCode != 0 {
		t.Errorf("Expected status 0 (valid), got %d", statusCode)
	}
	if errResp != nil {
		t.Errorf("Expected no error response, got %+v", errResp)
	}
}
