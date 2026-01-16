package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"telnyx-mock/internal/database"
	"telnyx-mock/internal/validator"
)

// HandleCreateMessage handles POST /v2/messages
func HandleCreateMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req validator.MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validator.WriteError(w, "10005", "Invalid parameter", "Invalid JSON payload.", http.StatusBadRequest)
		return
	}

	// Validate the request
	statusCode, errResp := validator.ValidateMessageRequest(r, &req)
	if errResp != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(errResp)
		return
	}

	// Generate UUID for message ID
	messageID := uuid.New().String()

	// Prepare media URLs
	mediaURLs := req.MediaURLs
	if mediaURLs == nil {
		mediaURLs = []string{}
	}

	// Insert into database
	if err := database.InsertMessage(messageID, req.From, req.To, req.Text, mediaURLs, req.MessagingProfileID, "outbound"); err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "Failed to save message.", http.StatusInternalServerError)
		return
	}

	// Return Telnyx success response format
	// Include all standard Telnyx response fields for API compatibility
	data := map[string]interface{}{
		"id":                  messageID,
		"record_type":         "message",
		"from":                req.From,
		"to":                  req.To,
		"text":                req.Text,
		"media_urls":          mediaURLs,
		"messaging_profile_id": req.MessagingProfileID,
		"direction":           "outbound",
		"status":              "queued", // Telnyx typically returns "queued" for new messages
		"created_at":          time.Now().UTC().Format(time.RFC3339),
		"updated_at":          time.Now().UTC().Format(time.RFC3339),
	}
	
	// Include webhook URLs only if provided in request (Telnyx omits empty fields)
	if req.WebhookURL != "" {
		data["webhook_url"] = req.WebhookURL
	}
	if req.WebhookFailoverURL != "" {
		data["webhook_failover_url"] = req.WebhookFailoverURL
	}
	if req.UseProfileWebhooks != nil {
		data["use_profile_webhooks"] = *req.UseProfileWebhooks
	}
	
	response := map[string]interface{}{
		"data": data,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleListMessages handles GET /api/messages
func HandleListMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	messages, err := database.GetAllMessages()
	if err != nil {
		http.Error(w, "Failed to retrieve messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// HandleClearMessages handles DELETE /api/messages
func HandleClearMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := database.ClearAllMessages(); err != nil {
		http.Error(w, "Failed to clear messages", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success"}`))
}

// HandleGetCredentials handles GET /api/credentials
func HandleGetCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cred, err := database.GetCredential()
	if err != nil {
		http.Error(w, "Failed to retrieve credentials", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cred)
}

// HandleSetCredentials handles POST /api/credentials
func HandleSetCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		APIKey string `json:"api_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		http.Error(w, "api_key is required", http.StatusBadRequest)
		return
	}

	if err := database.SetCredential(req.APIKey); err != nil {
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	cred, err := database.GetCredential()
	if err != nil {
		http.Error(w, "Failed to retrieve updated credentials", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cred)
}

// InboundWebhookPayload represents the Telnyx webhook payload for inbound messages
type InboundWebhookPayload struct {
	Data struct {
		EventType string `json:"event_type"`
		Payload   struct {
			ID                 string   `json:"id"`
			From               string   `json:"from"`
			To                 string   `json:"to"`
			Text               string   `json:"text"`
			MediaURLs          []string `json:"media_urls"`
			MessagingProfileID string   `json:"messaging_profile_id"`
			Direction          string   `json:"direction"`
		} `json:"payload"`
	} `json:"data"`
}

// HandleInboundWebhook handles POST /v2/webhooks/messages (Telnyx webhook format)
func HandleInboundWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read body once
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Try Telnyx webhook format first
	var webhook InboundWebhookPayload
	if err := json.Unmarshal(bodyBytes, &webhook); err == nil && webhook.Data.Payload.From != "" {
		// Handle Telnyx webhook format
		messageID := webhook.Data.Payload.ID
		if messageID == "" {
			messageID = uuid.New().String()
		}

		from := webhook.Data.Payload.From
		to := webhook.Data.Payload.To
		text := webhook.Data.Payload.Text
		mediaURLs := webhook.Data.Payload.MediaURLs
		messagingProfileID := webhook.Data.Payload.MessagingProfileID
		if mediaURLs == nil {
			mediaURLs = []string{}
		}

		if err := database.InsertMessage(messageID, from, to, text, mediaURLs, messagingProfileID, "inbound"); err != nil {
			http.Error(w, "Failed to save message", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
		return
	}

	// Try simpler format
	var simpleReq validator.MessageRequest
	if err := json.Unmarshal(bodyBytes, &simpleReq); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if simpleReq.From == "" || simpleReq.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}

	// Use simple format
	messageID := uuid.New().String()
	mediaURLs := simpleReq.MediaURLs
	messagingProfileID := simpleReq.MessagingProfileID
	if mediaURLs == nil {
		mediaURLs = []string{}
	}
	if err := database.InsertMessage(messageID, simpleReq.From, simpleReq.To, simpleReq.Text, mediaURLs, messagingProfileID, "inbound"); err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "received"}`))
}

// HandleSimulateInbound handles POST /api/messages/inbound (for UI simulation)
func HandleSimulateInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		From               string   `json:"from"`
		To                 string   `json:"to"`
		Text               string   `json:"text"`
		MediaURLs          []string `json:"media_urls"`
		MessagingProfileID string   `json:"messaging_profile_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Basic validation
	if req.From == "" || req.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}

	if req.Text == "" && (req.MediaURLs == nil || len(req.MediaURLs) == 0) {
		http.Error(w, "text or media_urls is required", http.StatusBadRequest)
		return
	}

	messageID := uuid.New().String()
	mediaURLs := req.MediaURLs
	messagingProfileID := req.MessagingProfileID
	if mediaURLs == nil {
		mediaURLs = []string{}
	}

	if err := database.InsertMessage(messageID, req.From, req.To, req.Text, mediaURLs, messagingProfileID, "inbound"); err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"id":        messageID,
		"from":      req.From,
		"to":        req.To,
		"text":      req.Text,
		"media_urls": mediaURLs,
		"direction": "inbound",
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
