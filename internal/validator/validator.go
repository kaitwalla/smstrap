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
type MessageRequest struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Text      string   `json:"text"`
	MediaURLs []string `json:"media_urls"`
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

	// Validate 'to' field
	if req.To == "" {
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

	// Type check: ensure 'to' and 'from' are strings (they should be from JSON unmarshaling)
	// If they're not strings, JSON unmarshaling would have failed already
	// But we can add explicit checks if needed

	return 0, nil // Valid request
}
