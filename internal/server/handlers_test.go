package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"telnyx-mock/internal/database"
)

func setupTestDB(t *testing.T) func() {
	testDBPath := "test_handlers.db"
	err := database.InitDB(testDBPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return func() {
		database.CloseDB()
		os.Remove(testDBPath)
	}
}

func TestHandleCreateMessage_Success(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"text":                  "Test message",
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' object")
	}

	// Check required fields
	if data["id"] == nil || data["id"] == "" {
		t.Error("Response should contain 'id'")
	}
	if data["record_type"] != "message" {
		t.Errorf("Expected record_type 'message', got '%v'", data["record_type"])
	}
	if data["direction"] != "outbound" {
		t.Errorf("Expected direction 'outbound', got '%v'", data["direction"])
	}
	
	// Check 'from' is now an object with phone_number
	fromObj, ok := data["from"].(map[string]interface{})
	if !ok {
		t.Error("Expected 'from' to be an object")
	} else if fromObj["phone_number"] != "+1234567890" {
		t.Errorf("Expected from.phone_number '+1234567890', got '%v'", fromObj["phone_number"])
	}
	
	// Check 'to' is now an array of recipient objects
	toArr, ok := data["to"].([]interface{})
	if !ok || len(toArr) == 0 {
		t.Error("Expected 'to' to be an array with at least one recipient")
	} else {
		toObj := toArr[0].(map[string]interface{})
		if toObj["phone_number"] != "+0987654321" {
			t.Errorf("Expected to[0].phone_number '+0987654321', got '%v'", toObj["phone_number"])
		}
		if toObj["status"] != "queued" {
			t.Errorf("Expected to[0].status 'queued', got '%v'", toObj["status"])
		}
	}
	
	if data["text"] != "Test message" {
		t.Errorf("Expected text 'Test message', got '%v'", data["text"])
	}
	if data["messaging_profile_id"] != "profile-123" {
		t.Errorf("Expected messaging_profile_id 'profile-123', got '%v'", data["messaging_profile_id"])
	}
	if data["type"] != "SMS" {
		t.Errorf("Expected type 'SMS', got '%v'", data["type"])
	}
}

func TestHandleCreateMessage_WithOptionalFields(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"text":                  "Test message",
		"messaging_profile_id":  "profile-123",
		"webhook_url":           "https://example.com/webhook",
		"webhook_failover_url": "https://example.com/failover",
		"use_profile_webhooks":  true,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})

	// Check optional fields are included in response
	if data["webhook_url"] != "https://example.com/webhook" {
		t.Errorf("Expected webhook_url in response, got '%v'", data["webhook_url"])
	}
	if data["webhook_failover_url"] != "https://example.com/failover" {
		t.Errorf("Expected webhook_failover_url in response, got '%v'", data["webhook_failover_url"])
	}
	if data["use_profile_webhooks"] != true {
		t.Errorf("Expected use_profile_webhooks true, got '%v'", data["use_profile_webhooks"])
	}
}

func TestHandleCreateMessage_ToAsArray(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Telnyx PHP SDK may send 'to' as an array
	body := map[string]interface{}{
		"from":                 "+1234567890",
		"to":                   []string{"+0987654321"},
		"text":                 "Test message",
		"messaging_profile_id": "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	toArr := data["to"].([]interface{})
	toObj := toArr[0].(map[string]interface{})
	if toObj["phone_number"] != "+0987654321" {
		t.Errorf("Expected to[0].phone_number '+0987654321', got '%v'", toObj["phone_number"])
	}
}

func TestHandleCreateMessage_MMS(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                 "+1234567890",
		"to":                   "+0987654321",
		"text":                 "Check out this image",
		"media_urls":           []string{"https://example.com/image.jpg"},
		"messaging_profile_id": "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	if data["type"] != "MMS" {
		t.Errorf("Expected type 'MMS' for message with media, got '%v'", data["type"])
	}
}

func TestHandleCreateMessage_MissingAuth(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"text":                  "Test message",
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	// No Authorization header
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	errors := response["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("Expected errors in response")
	}

	errObj := errors[0].(map[string]interface{})
	if errObj["code"] != "10001" {
		t.Errorf("Expected error code '10001', got '%v'", errObj["code"])
	}
}

func TestHandleCreateMessage_InvalidAuth(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"text":                  "Test message",
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestHandleCreateMessage_MissingFrom(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"to":                   "+0987654321",
		"text":                 "Test message",
		"messaging_profile_id": "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	// 'from' is now optional - should succeed with a default value
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	data := response["data"].(map[string]interface{})
	fromObj := data["from"].(map[string]interface{})
	// Verify 'from' was populated with a default based on profile
	if fromObj["phone_number"] == "" {
		t.Error("Expected 'from' to be populated with a default value")
	}
}

func TestHandleCreateMessage_MissingTo(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"text":                  "Test message",
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, rr.Code)
	}
}

func TestHandleCreateMessage_MissingMessagingProfileID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from": "+1234567890",
		"to":   "+0987654321",
		"text": "Test message",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	errors := response["errors"].([]interface{})
	errObj := errors[0].(map[string]interface{})
	if errObj["detail"] != "[SmsSink] The 'messaging_profile_id' parameter is required." {
		t.Errorf("Expected messaging_profile_id error, got '%v'", errObj["detail"])
	}
}

func TestHandleCreateMessage_MissingTextAndMediaURLs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("Expected status %d, got %d", http.StatusUnprocessableEntity, rr.Code)
	}
}

func TestHandleCreateMessage_WithMediaURLsNoText(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from":                  "+1234567890",
		"to":                    "+0987654321",
		"media_urls":            []string{"https://example.com/image.jpg"},
		"messaging_profile_id":  "profile-123",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}
}

func TestHandleCreateMessage_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/v2/messages", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleCreateMessage(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleListMessages(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a test message
	database.InsertMessage("test-id", "+111", "+222", "test", []string{}, "profile-1", "outbound")

	req := httptest.NewRequest(http.MethodGet, "/api/messages", nil)
	rr := httptest.NewRecorder()
	HandleListMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var messages []map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &messages)

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestHandleListMessages_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/messages", nil)
	rr := httptest.NewRecorder()
	HandleListMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Should return empty array, not null
	if rr.Body.String() != "[]\n" {
		t.Errorf("Expected '[]', got '%s'", rr.Body.String())
	}
}

func TestHandleClearMessages(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Insert some messages
	database.InsertMessage("id-1", "+111", "+222", "msg1", []string{}, "profile-1", "outbound")
	database.InsertMessage("id-2", "+333", "+444", "msg2", []string{}, "profile-2", "inbound")

	req := httptest.NewRequest(http.MethodDelete, "/api/messages", nil)
	rr := httptest.NewRecorder()
	HandleClearMessages(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify messages are cleared
	messages, _ := database.GetAllMessages()
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(messages))
	}
}

func TestHandleGetCredentials(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/credentials", nil)
	rr := httptest.NewRecorder()
	HandleGetCredentials(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var cred map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &cred)

	if cred["api_key"] != "test-token" {
		t.Errorf("Expected api_key 'test-token', got '%v'", cred["api_key"])
	}
}

func TestHandleSetCredentials(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]string{"api_key": "new-api-key"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/credentials", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleSetCredentials(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	// Verify the credential was updated
	cred, _ := database.GetCredential()
	if cred.APIKey != "new-api-key" {
		t.Errorf("Expected API key 'new-api-key', got '%s'", cred.APIKey)
	}
}

func TestHandleInboundWebhook_SimpleFormat(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from": "+1234567890",
		"to":   "+0987654321",
		"text": "Inbound message",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v2/webhooks/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleInboundWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	// Verify message was saved
	messages, _ := database.GetAllMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0].Direction != "inbound" {
		t.Errorf("Expected direction 'inbound', got '%s'", messages[0].Direction)
	}
}

func TestHandleSimulateInbound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	body := map[string]interface{}{
		"from": "+1234567890",
		"to":   "+0987654321",
		"text": "Simulated inbound",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/messages/inbound", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleSimulateInbound(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	if response["direction"] != "inbound" {
		t.Errorf("Expected direction 'inbound', got '%v'", response["direction"])
	}
}

func TestMethodNotAllowed(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		handler func(http.ResponseWriter, *http.Request)
		method  string
		path    string
	}{
		{HandleCreateMessage, http.MethodGet, "/v2/messages"},
		{HandleListMessages, http.MethodPost, "/api/messages"},
		{HandleClearMessages, http.MethodGet, "/api/messages"},
		{HandleGetCredentials, http.MethodPost, "/api/credentials"},
		{HandleSetCredentials, http.MethodGet, "/api/credentials"},
		{HandleInboundWebhook, http.MethodGet, "/v2/webhooks/messages"},
		{HandleSimulateInbound, http.MethodGet, "/api/messages/inbound"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		tc.handler(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: Expected status %d, got %d", tc.method, tc.path, http.StatusMethodNotAllowed, rr.Code)
		}
	}
}
