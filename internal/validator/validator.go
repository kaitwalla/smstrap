package validator

import (
	"encoding/json"
	"net/http"

	"telnyx-mock/internal/database"
)

// TelnyxError represents a single error in Telnyx error response format
type TelnyxError struct {
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// TelnyxErrorResponse represents the standard Telnyx error response format
type TelnyxErrorResponse struct {
	Errors []TelnyxError `json:"errors"`
}

// MessageRequest represents the incoming message request payload
// Matches Telnyx API v2/messages request format
// Note: Telnyx accepts "to" as either a string "+1234567890" or an array ["+1234567890"]
type MessageRequest struct {
	From               string   `json:"from"`
	To                 string   `json:"-"` // Handled by custom unmarshal
	ToRaw              any      `json:"to"`
	Text               string   `json:"text"`
	MediaURLs          []string `json:"media_urls"`
	MessagingProfileID string   `json:"messaging_profile_id"`
	WebhookURL         string   `json:"webhook_url,omitempty"`
	WebhookFailoverURL string   `json:"webhook_failover_url,omitempty"`
	UseProfileWebhooks *bool    `json:"use_profile_webhooks,omitempty"`
	// Additional optional Telnyx fields for API compatibility
	Type           string `json:"type,omitempty"`            // "SMS" or "MMS"
	Subject        string `json:"subject,omitempty"`         // MMS subject
	AutoDetect     *bool  `json:"auto_detect,omitempty"`     // Auto-detect encoding
}

// NormalizeTo extracts the phone number from the To field
// Telnyx accepts "to" as a string OR an array of strings
func (m *MessageRequest) NormalizeTo() string {
	if m.To != "" {
		return m.To
	}

	if m.ToRaw == nil {
		return ""
	}

	// Handle string
	if s, ok := m.ToRaw.(string); ok {
		m.To = s
		return s
	}

	// Handle array of strings
	if arr, ok := m.ToRaw.([]interface{}); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			m.To = s
			return s
		}
	}

	return ""
}

// WriteError writes a Telnyx-formatted error response
func WriteError(w http.ResponseWriter, code, title, detail string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := TelnyxErrorResponse{
		Errors: []TelnyxError{
			{
				Code:   code,
				Title:  title,
				Detail: detail,
			},
		},
	}

	json.NewEncoder(w).Encode(response)
}

// ValidateMessageRequest performs strict validation on the message request
// Returns nil if valid, or an error response that should be written
func ValidateMessageRequest(r *http.Request, req *MessageRequest) (int, *TelnyxErrorResponse) {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return http.StatusUnauthorized, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10001",
					Title:  "Unauthorized",
					Detail: "Authorization header is required.",
				},
			},
		}
	}

	// Validate against stored credentials
	if !database.ValidateCredential(authHeader) {
		return http.StatusUnauthorized, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10001",
					Title:  "Unauthorized",
					Detail: "Invalid API key.",
				},
			},
		}
	}

	// Validate 'from' field
	if req.From == "" {
		return http.StatusUnprocessableEntity, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10005",
					Title:  "Invalid parameter",
					Detail: "The 'from' parameter is required.",
				},
			},
		}
	}

	// Normalize and validate 'to' field (handles string or array)
	to := req.NormalizeTo()
	if to == "" {
		return http.StatusUnprocessableEntity, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10005",
					Title:  "Invalid parameter",
					Detail: "The 'to' parameter is required.",
				},
			},
		}
	}

	// Validate 'messaging_profile_id' field
	if req.MessagingProfileID == "" {
		return http.StatusUnprocessableEntity, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10005",
					Title:  "Invalid parameter",
					Detail: "The 'messaging_profile_id' parameter is required.",
				},
			},
		}
	}

	// Validate that at least one of 'text' or 'media_urls' is present
	if req.Text == "" && (req.MediaURLs == nil || len(req.MediaURLs) == 0) {
		return http.StatusUnprocessableEntity, &TelnyxErrorResponse{
			Errors: []TelnyxError{
				{
					Code:   "10005",
					Title:  "Invalid parameter",
					Detail: "Either 'text' or 'media_urls' parameter is required.",
				},
			},
		}
	}

	return 0, nil // Valid request
}
