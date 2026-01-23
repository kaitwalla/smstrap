package webhook

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"telnyx-mock/internal/database"
)

// MessageDetails contains info needed for webhook callbacks
type MessageDetails struct {
	ID                 string
	From               string
	To                 string
	Text               string
	MediaURLs          []string
	MessagingProfileID string
	Type               string
	WebhookURL         string
	WebhookFailoverURL string
}

// TelnyxWebhookPayload represents the standard Telnyx webhook format
type TelnyxWebhookPayload struct {
	Data TelnyxWebhookData `json:"data"`
}

// TelnyxWebhookData contains the webhook event data
type TelnyxWebhookData struct {
	EventType  string                 `json:"event_type"`
	ID         string                 `json:"id"`
	OccurredAt string                 `json:"occurred_at"`
	Payload    map[string]interface{} `json:"payload"`
	RecordType string                 `json:"record_type"`
}

// SendStatusCallbacks sends a series of status webhooks simulating message delivery
// Telnyx sends: message.queued → message.sent → message.delivered (or message.failed)
func SendStatusCallbacks(msg MessageDetails) {
	if msg.WebhookURL == "" {
		return
	}

	go func() {
		now := time.Now().UTC()

		// Build base payload
		basePayload := map[string]interface{}{
			"id":                   msg.ID,
			"record_type":          "message",
			"direction":            "outbound",
			"messaging_profile_id": msg.MessagingProfileID,
			"from": map[string]interface{}{
				"phone_number": msg.From,
				"carrier":      "SmsSink Mock Carrier",
				"line_type":    "Wireless",
			},
			"to": []map[string]interface{}{
				{
					"phone_number": msg.To,
					"carrier":      "SmsSink Mock Carrier",
					"line_type":    "Wireless",
				},
			},
			"text":  msg.Text,
			"media": msg.MediaURLs,
			"type":  msg.Type,
		}

		// Status sequence with delays to simulate real-world timing
		statuses := []struct {
			eventType string
			status    string
			delay     time.Duration
		}{
			{"message.sent", "sent", 500 * time.Millisecond},
			{"message.delivered", "delivered", 1500 * time.Millisecond},
		}

		for _, s := range statuses {
			time.Sleep(s.delay)

			payload := copyMap(basePayload)
			payload["status"] = s.status

			// Add timestamps based on status
			occurredAt := now.Add(s.delay).Format(time.RFC3339)
			switch s.status {
			case "sent":
				payload["sent_at"] = occurredAt
			case "delivered":
				payload["sent_at"] = now.Add(500 * time.Millisecond).Format(time.RFC3339)
				payload["completed_at"] = occurredAt
			}

			// Update the to array status
			if toArr, ok := payload["to"].([]map[string]interface{}); ok && len(toArr) > 0 {
				toArr[0]["status"] = s.status
			}

			webhookPayload := TelnyxWebhookPayload{
				Data: TelnyxWebhookData{
					EventType:  s.eventType,
					ID:         uuid.New().String(),
					OccurredAt: occurredAt,
					Payload:    payload,
					RecordType: "event",
				},
			}

			sendWebhook(msg.WebhookURL, msg.WebhookFailoverURL, webhookPayload)
		}
	}()
}

// sendWebhook sends a webhook to the specified URL
func sendWebhook(url, failoverURL string, payload TelnyxWebhookPayload) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Webhook: Failed to marshal payload: %v", err)
		database.LogError("webhook", "Failed to marshal webhook payload", map[string]interface{}{
			"error":      err.Error(),
			"event_type": payload.Data.EventType,
		})
		return
	}

	messageID, _ := payload.Data.Payload["id"].(string)

	// Try primary URL
	if err := doWebhookRequest(url, body); err != nil {
		log.Printf("Webhook: Primary URL failed (%s): %v", url, err)
		database.LogWarning("webhook", "Primary webhook URL failed", map[string]interface{}{
			"url":        url,
			"error":      err.Error(),
			"event_type": payload.Data.EventType,
			"message_id": messageID,
		})

		// Try failover URL if available
		if failoverURL != "" {
			if err := doWebhookRequest(failoverURL, body); err != nil {
				log.Printf("Webhook: Failover URL also failed (%s): %v", failoverURL, err)
				database.LogError("webhook", "Failover webhook URL also failed", map[string]interface{}{
					"url":        failoverURL,
					"error":      err.Error(),
					"event_type": payload.Data.EventType,
					"message_id": messageID,
				})
			} else {
				log.Printf("Webhook: Sent to failover URL: %s (event: %s)", failoverURL, payload.Data.EventType)
				database.Log("webhook", "Webhook sent to failover URL", map[string]interface{}{
					"url":        failoverURL,
					"event_type": payload.Data.EventType,
					"message_id": messageID,
				})
			}
		}
	} else {
		log.Printf("Webhook: Sent to %s (event: %s, message: %s)", url, payload.Data.EventType, payload.Data.Payload["id"])
		database.Log("webhook", "Webhook sent successfully", map[string]interface{}{
			"url":        url,
			"event_type": payload.Data.EventType,
			"message_id": messageID,
		})
	}
}

// doWebhookRequest performs the actual HTTP request
func doWebhookRequest(url string, body []byte) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SmsSink/1.0")

	// Telnyx includes these headers - we'll add placeholders
	req.Header.Set("telnyx-timestamp", time.Now().UTC().Format(time.RFC3339))
	req.Header.Set("telnyx-signature-ed25519", "mock-signature")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Telnyx expects 2xx response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &WebhookError{StatusCode: resp.StatusCode}
	}

	return nil
}

// WebhookError represents a webhook delivery failure
type WebhookError struct {
	StatusCode int
}

func (e *WebhookError) Error() string {
	return "webhook returned non-2xx status"
}

// copyMap creates a shallow copy of a map
func copyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}
