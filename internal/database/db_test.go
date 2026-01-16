package database

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) func() {
	testDBPath := "test_smssink.db"
	err := InitDB(testDBPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return func() {
		CloseDB()
		os.Remove(testDBPath)
	}
}

func TestInitDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if DB == nil {
		t.Error("Database should be initialized")
	}
}

func TestInsertAndGetMessage(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a message
	err := InsertMessage(
		"test-id-123",
		"+1234567890",
		"+0987654321",
		"Test message content",
		[]string{"https://example.com/image.jpg"},
		"profile-123",
		"outbound",
	)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	// Get all messages
	messages, err := GetAllMessages()
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.ID != "test-id-123" {
		t.Errorf("Expected ID 'test-id-123', got '%s'", msg.ID)
	}
	if msg.Sender != "+1234567890" {
		t.Errorf("Expected sender '+1234567890', got '%s'", msg.Sender)
	}
	if msg.Recipient != "+0987654321" {
		t.Errorf("Expected recipient '+0987654321', got '%s'", msg.Recipient)
	}
	if msg.Content != "Test message content" {
		t.Errorf("Expected content 'Test message content', got '%s'", msg.Content)
	}
	if msg.Direction != "outbound" {
		t.Errorf("Expected direction 'outbound', got '%s'", msg.Direction)
	}
	if msg.MessagingProfileID != "profile-123" {
		t.Errorf("Expected messaging_profile_id 'profile-123', got '%s'", msg.MessagingProfileID)
	}
}

func TestInsertMessageWithEmptyMediaURLs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	err := InsertMessage(
		"test-id-456",
		"+1234567890",
		"+0987654321",
		"Test message",
		[]string{},
		"profile-123",
		"outbound",
	)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}

	messages, err := GetAllMessages()
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].MediaURLs != "[]" {
		t.Errorf("Expected media_urls '[]', got '%s'", messages[0].MediaURLs)
	}
}

func TestClearAllMessages(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert some messages
	InsertMessage("id-1", "+111", "+222", "msg1", []string{}, "profile-1", "outbound")
	InsertMessage("id-2", "+333", "+444", "msg2", []string{}, "profile-2", "inbound")

	// Verify messages exist
	messages, _ := GetAllMessages()
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages before clear, got %d", len(messages))
	}

	// Clear messages
	err := ClearAllMessages()
	if err != nil {
		t.Fatalf("Failed to clear messages: %v", err)
	}

	// Verify messages are cleared
	messages, _ = GetAllMessages()
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(messages))
	}
}

func TestGetAllMessagesEmpty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	messages, err := GetAllMessages()
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	// Should return empty slice, not nil
	if messages == nil {
		t.Error("Expected empty slice, got nil")
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}
}

func TestDefaultCredential(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	cred, err := GetCredential()
	if err != nil {
		t.Fatalf("Failed to get credential: %v", err)
	}

	if cred.APIKey != "test-token" {
		t.Errorf("Expected default API key 'test-token', got '%s'", cred.APIKey)
	}
}

func TestSetAndGetCredential(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Set new credential
	err := SetCredential("new-api-key-123")
	if err != nil {
		t.Fatalf("Failed to set credential: %v", err)
	}

	// Get credential
	cred, err := GetCredential()
	if err != nil {
		t.Fatalf("Failed to get credential: %v", err)
	}

	if cred.APIKey != "new-api-key-123" {
		t.Errorf("Expected API key 'new-api-key-123', got '%s'", cred.APIKey)
	}
}

func TestValidateCredential_BearerFormat(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Test with Bearer prefix
	if !ValidateCredential("Bearer test-token") {
		t.Error("Should validate 'Bearer test-token'")
	}
}

func TestValidateCredential_DirectFormat(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Test without Bearer prefix
	if !ValidateCredential("test-token") {
		t.Error("Should validate 'test-token' without Bearer prefix")
	}
}

func TestValidateCredential_Invalid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if ValidateCredential("wrong-token") {
		t.Error("Should not validate 'wrong-token'")
	}

	if ValidateCredential("Bearer wrong-token") {
		t.Error("Should not validate 'Bearer wrong-token'")
	}
}

func TestMessagesOrderedByCreatedAtDesc(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert messages in order
	InsertMessage("id-first", "+111", "+222", "first", []string{}, "profile-1", "outbound")
	InsertMessage("id-second", "+333", "+444", "second", []string{}, "profile-2", "outbound")
	InsertMessage("id-third", "+555", "+666", "third", []string{}, "profile-3", "outbound")

	messages, err := GetAllMessages()
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}

	// Most recent should be first (DESC order)
	if messages[0].ID != "id-third" {
		t.Errorf("Expected first message to be 'id-third', got '%s'", messages[0].ID)
	}
	if messages[2].ID != "id-first" {
		t.Errorf("Expected last message to be 'id-first', got '%s'", messages[2].ID)
	}
}
