package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"telnyx-mock/internal/database"
	"telnyx-mock/internal/validator"
	"telnyx-mock/internal/webhook"
)

// isDebugMode checks if debug mode is enabled (env var or database setting)
func isDebugMode() bool {
	// Environment variable takes precedence
	if os.Getenv("SMSSINK_DEBUG") == "true" {
		return true
	}
	// Otherwise check database setting
	return database.IsDebugMode()
}

// HandleCreateMessage handles POST /v2/messages
func HandleCreateMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only POST method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	// Read body for parsing
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Failed to read request body.", http.StatusBadRequest)
		return
	}

	// Log raw request body only in debug mode
	if isDebugMode() {
		database.Log("message", "Raw request body received", map[string]interface{}{
			"body":       string(bodyBytes),
			"ip":         r.RemoteAddr,
			"user_agent": r.UserAgent(),
		})
	}

	var req validator.MessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		errMsg := err.Error()
		database.LogError("message", "Invalid JSON payload in outbound message request", map[string]interface{}{
			"error":      errMsg,
			"ip":         r.RemoteAddr,
			"user_agent": r.UserAgent(),
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Invalid JSON payload: "+errMsg, http.StatusBadRequest)
		return
	}

	// Validate the request
	statusCode, errResp := validator.ValidateMessageRequest(r, &req)
	if errResp != nil {
		database.LogError("message", "Validation failed for outbound message", map[string]interface{}{
			"status_code": statusCode,
			"from":        req.From,
			"to":          req.NormalizeTo(),
			"ip":          r.RemoteAddr,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(errResp)
		return
	}

	// Get normalized 'to' value (handles both string and array formats)
	to := req.NormalizeTo()

	// Generate UUID for message ID
	messageID := uuid.New().String()

	// Prepare media URLs
	mediaURLs := req.MediaURLs
	if mediaURLs == nil {
		mediaURLs = []string{}
	}

	// Determine message type
	msgType := "SMS"
	if len(mediaURLs) > 0 {
		msgType = "MMS"
	}

	// Insert into database
	if err := database.InsertMessage(messageID, req.From, to, req.Text, mediaURLs, req.MessagingProfileID, "outbound"); err != nil {
		database.LogError("message", "Failed to save outbound message to database", map[string]interface{}{
			"error": err.Error(),
			"from":  req.From,
			"to":    to,
		})
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save message.", http.StatusInternalServerError)
		return
	}

	// Log successful outbound message
	database.Log("message", "Outbound message sent successfully", map[string]interface{}{
		"message_id": messageID,
		"from":       req.From,
		"to":         to,
		"type":       msgType,
		"has_text":   req.Text != "",
		"media_count": len(mediaURLs),
	})

	now := time.Now().UTC()

	// Return Telnyx success response format
	// Include all standard Telnyx response fields for API compatibility
	// The 'to' field in responses is an array of recipient objects
	data := map[string]interface{}{
		"id":                   messageID,
		"record_type":          "message",
		"direction":            "outbound",
		"messaging_profile_id": req.MessagingProfileID,
		"from": map[string]interface{}{
			"phone_number": req.From,
			"carrier":      "",
			"line_type":    "",
		},
		"to": []map[string]interface{}{
			{
				"phone_number": to,
				"status":       "queued",
				"carrier":      "",
				"line_type":    "",
			},
		},
		"text":       req.Text,
		"media":      mediaURLs, // Telnyx uses 'media' in responses
		"type":       msgType,
		"valid_until": now.Add(24 * time.Hour).Format(time.RFC3339),
		"webhook_url":          "",
		"webhook_failover_url": "",
		"encoding":             "GSM-7",
		"parts":                1,
		"tags":                 []string{},
		"cost":                 nil,
		"received_at":          nil,
		"sent_at":              nil,
		"completed_at":         nil,
		"created_at":           now.Format(time.RFC3339),
		"updated_at":           now.Format(time.RFC3339),
	}

	// Include webhook URLs if provided in request
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

	// Send status callbacks asynchronously if webhook URL is provided
	if req.WebhookURL != "" {
		webhook.SendStatusCallbacks(webhook.MessageDetails{
			ID:                 messageID,
			From:               req.From,
			To:                 to,
			Text:               req.Text,
			MediaURLs:          mediaURLs,
			MessagingProfileID: req.MessagingProfileID,
			Type:               msgType,
			WebhookURL:         req.WebhookURL,
			WebhookFailoverURL: req.WebhookFailoverURL,
		})
	}
}

// HandleListMessages handles GET /api/messages
func HandleListMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only GET method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	messages, err := database.GetAllMessages()
	if err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to retrieve messages.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// HandleClearMessages handles DELETE /api/messages
func HandleClearMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only DELETE method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	if err := database.ClearAllMessages(); err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to clear messages.", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success"}`))
}

// HandleGetCredentials handles GET /api/credentials
func HandleGetCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only GET method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	cred, err := database.GetCredential()
	if err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to retrieve credentials.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cred)
}

// HandleSetCredentials handles POST /api/credentials
func HandleSetCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only POST method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		APIKey string `json:"api_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Invalid JSON payload.", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] The 'api_key' parameter is required.", http.StatusBadRequest)
		return
	}

	if err := database.SetCredential(req.APIKey); err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save credentials.", http.StatusInternalServerError)
		return
	}

	cred, err := database.GetCredential()
	if err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to retrieve updated credentials.", http.StatusInternalServerError)
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
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only POST method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	// Read body once
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		database.LogError("webhook", "Failed to read webhook request body", map[string]interface{}{
			"error": err.Error(),
			"ip":    r.RemoteAddr,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Failed to read request body.", http.StatusBadRequest)
		return
	}

	// Try Telnyx webhook format first
	var webhookPayload InboundWebhookPayload
	if err := json.Unmarshal(bodyBytes, &webhookPayload); err == nil && webhookPayload.Data.Payload.From != "" {
		// Handle Telnyx webhook format
		messageID := webhookPayload.Data.Payload.ID
		if messageID == "" {
			messageID = uuid.New().String()
		}

		from := webhookPayload.Data.Payload.From
		to := webhookPayload.Data.Payload.To
		text := webhookPayload.Data.Payload.Text
		mediaURLs := webhookPayload.Data.Payload.MediaURLs
		messagingProfileID := webhookPayload.Data.Payload.MessagingProfileID
		if mediaURLs == nil {
			mediaURLs = []string{}
		}

		if err := database.InsertMessage(messageID, from, to, text, mediaURLs, messagingProfileID, "inbound"); err != nil {
			database.LogError("webhook", "Failed to save inbound webhook message", map[string]interface{}{
				"error":      err.Error(),
				"message_id": messageID,
				"from":       from,
				"to":         to,
			})
			validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save message.", http.StatusInternalServerError)
			return
		}

		database.Log("webhook", "Inbound message received via Telnyx webhook", map[string]interface{}{
			"message_id":  messageID,
			"from":        from,
			"to":          to,
			"event_type":  webhookPayload.Data.EventType,
			"media_count": len(mediaURLs),
		})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
		return
	}

	// Try simpler format
	var simpleReq validator.MessageRequest
	if err := json.Unmarshal(bodyBytes, &simpleReq); err != nil {
		errMsg := err.Error()
		database.LogError("webhook", "Invalid JSON payload in webhook", map[string]interface{}{
			"error": errMsg,
			"ip":    r.RemoteAddr,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Invalid JSON payload: "+errMsg, http.StatusBadRequest)
		return
	}

	// Normalize 'to' field (handles string or array)
	to := simpleReq.NormalizeTo()

	// Validate required fields
	if simpleReq.From == "" || to == "" {
		database.LogError("webhook", "Missing required fields in webhook", map[string]interface{}{
			"from": simpleReq.From,
			"to":   to,
			"ip":   r.RemoteAddr,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] The 'from' and 'to' parameters are required.", http.StatusBadRequest)
		return
	}

	// Use simple format
	messageID := uuid.New().String()
	mediaURLs := simpleReq.MediaURLs
	messagingProfileID := simpleReq.MessagingProfileID
	if mediaURLs == nil {
		mediaURLs = []string{}
	}
	if err := database.InsertMessage(messageID, simpleReq.From, to, simpleReq.Text, mediaURLs, messagingProfileID, "inbound"); err != nil {
		database.LogError("webhook", "Failed to save inbound message (simple format)", map[string]interface{}{
			"error":      err.Error(),
			"message_id": messageID,
			"from":       simpleReq.From,
			"to":         to,
		})
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save message.", http.StatusInternalServerError)
		return
	}

	database.Log("webhook", "Inbound message received via simple webhook", map[string]interface{}{
		"message_id":  messageID,
		"from":        simpleReq.From,
		"to":          to,
		"media_count": len(mediaURLs),
	})

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "received"}`))
}

// HandleSimulateInbound handles POST /api/messages/inbound (for UI simulation)
func HandleSimulateInbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only POST method is supported for this endpoint.", http.StatusMethodNotAllowed)
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
		errMsg := err.Error()
		database.LogError("message", "Invalid JSON payload in simulate inbound", map[string]interface{}{
			"error": errMsg,
			"ip":    r.RemoteAddr,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Invalid JSON payload: "+errMsg, http.StatusBadRequest)
		return
	}

	// Basic validation
	if req.From == "" || req.To == "" {
		database.LogError("message", "Missing required fields in simulate inbound", map[string]interface{}{
			"from": req.From,
			"to":   req.To,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] The 'from' and 'to' parameters are required.", http.StatusBadRequest)
		return
	}

	if req.Text == "" && len(req.MediaURLs) == 0 {
		database.LogError("message", "Missing text or media_urls in simulate inbound", map[string]interface{}{
			"from": req.From,
			"to":   req.To,
		})
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Either 'text' or 'media_urls' parameter is required.", http.StatusBadRequest)
		return
	}

	messageID := uuid.New().String()
	mediaURLs := req.MediaURLs
	messagingProfileID := req.MessagingProfileID
	if mediaURLs == nil {
		mediaURLs = []string{}
	}

	if err := database.InsertMessage(messageID, req.From, req.To, req.Text, mediaURLs, messagingProfileID, "inbound"); err != nil {
		database.LogError("message", "Failed to save simulated inbound message", map[string]interface{}{
			"error":      err.Error(),
			"message_id": messageID,
			"from":       req.From,
			"to":         req.To,
		})
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save message.", http.StatusInternalServerError)
		return
	}

	database.Log("message", "Simulated inbound message created", map[string]interface{}{
		"message_id":  messageID,
		"from":        req.From,
		"to":          req.To,
		"media_count": len(mediaURLs),
	})

	response := map[string]interface{}{
		"id":         messageID,
		"from":       req.From,
		"to":         req.To,
		"text":       req.Text,
		"media_urls": mediaURLs,
		"direction":  "inbound",
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleGetLogs handles GET /api/logs
func HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only GET method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	level := r.URL.Query().Get("level")
	category := r.URL.Query().Get("category")
	limitStr := r.URL.Query().Get("limit")

	limit := 100
	if limitStr != "" {
		if parsed, err := parseLimit(limitStr); err == nil {
			limit = parsed
		}
	}

	logs, err := database.GetLogs(level, category, limit)
	if err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to retrieve logs.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// HandleClearLogs handles DELETE /api/logs
func HandleClearLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only DELETE method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	if err := database.ClearAllLogs(); err != nil {
		validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to clear logs.", http.StatusInternalServerError)
		return
	}

	database.Log("system", "All logs cleared", nil)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success"}`))
}

// parseLimit safely parses a limit string to int
func parseLimit(s string) (int, error) {
	var limit int
	err := json.Unmarshal([]byte(s), &limit)
	if err != nil {
		return 0, err
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return limit, nil
}

// HandleGetSettings handles GET /api/settings
func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only GET method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	debugMode := database.IsDebugMode()

	response := map[string]interface{}{
		"debug_mode": debugMode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleSetSettings handles POST /api/settings
func HandleSetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		validator.WriteError(w, "10003", "Method not allowed", "[SmsSink] Only POST method is supported for this endpoint.", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DebugMode *bool `json:"debug_mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validator.WriteError(w, "10005", "Invalid parameter", "[SmsSink] Invalid JSON payload.", http.StatusBadRequest)
		return
	}

	if req.DebugMode != nil {
		value := "false"
		if *req.DebugMode {
			value = "true"
		}
		if err := database.SetSetting("debug_mode", value); err != nil {
			validator.WriteError(w, "10000", "Internal Server Error", "[SmsSink] Failed to save settings.", http.StatusInternalServerError)
			return
		}
		database.Log("system", "Debug mode changed", map[string]interface{}{
			"debug_mode": *req.DebugMode,
		})
	}

	// Return updated settings
	debugMode := database.IsDebugMode()
	response := map[string]interface{}{
		"debug_mode": debugMode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
