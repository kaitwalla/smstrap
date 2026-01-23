package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSendStatusCallbacks_NoWebhookURL(t *testing.T) {
	// Should not panic or cause issues when webhook URL is empty
	msg := MessageDetails{
		ID:                 "test-id",
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Test message",
		MessagingProfileID: "profile-123",
		Type:               "SMS",
		WebhookURL:         "", // Empty - no webhooks should be sent
	}

	// This should return immediately without doing anything
	SendStatusCallbacks(msg)
	
	// Give it a moment to ensure no panic
	time.Sleep(100 * time.Millisecond)
}

func TestSendStatusCallbacks_SendsWebhooks(t *testing.T) {
	var mu sync.Mutex
	receivedEvents := []string{}

	// Create a test server to receive webhooks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload TelnyxWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode webhook payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedEvents = append(receivedEvents, payload.Data.EventType)
		mu.Unlock()

		// Check required headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json")
		}
		if r.Header.Get("telnyx-timestamp") == "" {
			t.Error("Expected telnyx-timestamp header")
		}
		if r.Header.Get("telnyx-signature-ed25519") == "" {
			t.Error("Expected telnyx-signature-ed25519 header")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msg := MessageDetails{
		ID:                 "test-id-123",
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Test message",
		MessagingProfileID: "profile-123",
		Type:               "SMS",
		WebhookURL:         server.URL,
	}

	SendStatusCallbacks(msg)

	// Wait for webhooks to be sent (they're async with delays)
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// Should receive: message.sent, message.delivered
	if len(receivedEvents) != 2 {
		t.Errorf("Expected 2 webhook events, got %d: %v", len(receivedEvents), receivedEvents)
	}

	expectedEvents := []string{"message.sent", "message.delivered"}
	for i, expected := range expectedEvents {
		if i < len(receivedEvents) && receivedEvents[i] != expected {
			t.Errorf("Expected event %d to be '%s', got '%s'", i, expected, receivedEvents[i])
		}
	}
}

func TestSendStatusCallbacks_FailoverURL(t *testing.T) {
	var mu sync.Mutex
	failoverHits := 0

	// Primary server that always fails
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer primaryServer.Close()

	// Failover server that succeeds
	failoverServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		failoverHits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer failoverServer.Close()

	msg := MessageDetails{
		ID:                 "test-id-456",
		From:               "+1234567890",
		To:                 "+0987654321",
		Text:               "Test message",
		MessagingProfileID: "profile-123",
		Type:               "SMS",
		WebhookURL:         primaryServer.URL,
		WebhookFailoverURL: failoverServer.URL,
	}

	SendStatusCallbacks(msg)

	// Wait for webhooks
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	// Should hit failover for both events
	if failoverHits != 2 {
		t.Errorf("Expected 2 failover hits, got %d", failoverHits)
	}
}

func TestWebhookPayloadStructure(t *testing.T) {
	var mu sync.Mutex
	var receivedPayload TelnyxWebhookPayload
	received := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload TelnyxWebhookPayload
		json.NewDecoder(r.Body).Decode(&payload)
		
		mu.Lock()
		receivedPayload = payload
		mu.Unlock()
		
		// Signal that we received the first webhook
		select {
		case received <- struct{}{}:
		default:
		}
		
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msg := MessageDetails{
		ID:                 "msg-abc-123",
		From:               "+15551234567",
		To:                 "+15559876543",
		Text:               "Hello, World!",
		MediaURLs:          []string{"https://example.com/image.jpg"},
		MessagingProfileID: "prof-xyz",
		Type:               "MMS",
		WebhookURL:         server.URL,
	}

	SendStatusCallbacks(msg)

	// Wait for first webhook with timeout
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for webhook")
	}

	// Give a small buffer for the handler to finish
	time.Sleep(50 * time.Millisecond)

	// Read with lock
	mu.Lock()
	payload := receivedPayload
	mu.Unlock()

	// Validate payload structure
	if payload.Data.RecordType != "event" {
		t.Errorf("Expected record_type 'event', got '%s'", payload.Data.RecordType)
	}

	if payload.Data.EventType != "message.sent" {
		t.Errorf("Expected first event 'message.sent', got '%s'", payload.Data.EventType)
	}

	data := payload.Data.Payload
	if data["id"] != "msg-abc-123" {
		t.Errorf("Expected message id 'msg-abc-123', got '%v'", data["id"])
	}

	if data["type"] != "MMS" {
		t.Errorf("Expected type 'MMS', got '%v'", data["type"])
	}

	if data["text"] != "Hello, World!" {
		t.Errorf("Expected text 'Hello, World!', got '%v'", data["text"])
	}

	// Check 'from' structure
	from, ok := data["from"].(map[string]interface{})
	if !ok {
		t.Error("Expected 'from' to be an object")
	} else if from["phone_number"] != "+15551234567" {
		t.Errorf("Expected from.phone_number '+15551234567', got '%v'", from["phone_number"])
	}

	// Check 'to' structure (JSON unmarshals to []interface{})
	toArr, ok := data["to"].([]interface{})
	if !ok || len(toArr) == 0 {
		t.Error("Expected 'to' to be an array with at least one element")
	} else {
		toObj := toArr[0].(map[string]interface{})
		if toObj["phone_number"] != "+15559876543" {
			t.Errorf("Expected to[0].phone_number '+15559876543', got '%v'", toObj["phone_number"])
		}
	}
}
