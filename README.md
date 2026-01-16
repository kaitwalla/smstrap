# SmsSink

A robust, single-binary Telnyx Messaging API mock server for local development and testing. SmsSink provides strict validation, SQLite persistence, credential management, and a web UI for inspecting both inbound and outbound messages.

## Features

- **Strict Validation**: Mimics Telnyx API validation rules, returning proper error responses (422 Unprocessable Entity) for invalid requests
- **Credential Management**: Configurable API keys with web-based credential management
- **Inbound & Outbound Messaging**: Full support for both sending and receiving messages
- **SQLite Persistence**: All messages are stored in a local SQLite database
- **Web UI**: Beautiful dashboard to view, filter, and manage message history
- **Webhook Support**: Endpoint to receive inbound messages (Telnyx webhook format)
- **Status Callbacks**: Automatic delivery status webhooks (message.sent, message.delivered) when webhook_url is provided
- **Single Binary**: No external dependencies, pure Go implementation with embedded assets
- **Dual Server**: API server (port 23456) and UI server (port 23457)

## Building

### For Linux (x86-64)

```bash
GOOS=linux GOARCH=amd64 go build -o SmsSink .
```

### For macOS (Apple Silicon)

```bash
go build -o SmsSink .
```

### For macOS (Intel)

```bash
GOOS=darwin GOARCH=amd64 go build -o SmsSink .
```

### For Windows

```bash
GOOS=windows GOARCH=amd64 go build -o SmsSink.exe .
```

The binary is statically linked and requires no external dependencies (CGO_ENABLED=0).

## Running

```bash
./SmsSink
```

The server will start two services:
- **API Server**: `http://localhost:23456` - Mock Telnyx API endpoint
- **Web UI**: `http://localhost:23457` - Message inspector dashboard

A SQLite database file (`smssink.db`) will be created automatically in the current directory.

## Quick Start

1. **Start the server**:
   ```bash
   ./SmsSink
   ```

2. **Configure API credentials**:
   - Open `http://localhost:23457/credentials`
   - Set your API key (default: `test-token`)

3. **Send a test message**:
   ```bash
   curl -X POST http://localhost:23456/v2/messages \
     -H "Authorization: Bearer test-token" \
     -H "Content-Type: application/json" \
     -d '{
       "from": "+1234567890",
       "to": "+0987654321",
       "text": "Hello from SmsSink!",
       "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
     }'
   ```

4. **View messages**:
   - Open `http://localhost:23457` to see all messages

## API Endpoints

### POST /v2/messages

Send an outbound message through the mock API.

**Headers:**
- `Authorization`: Required (must match configured API key)
- `Content-Type`: `application/json`

**Request Body:**
```json
{
  "from": "+1234567890",
  "to": "+0987654321",
  "text": "Hello, world!",
  "media_urls": ["https://example.com/image.jpg"],
  "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

**Validation Rules:**
- `from`: Required (string)
- `to`: Required (string)
- `messaging_profile_id`: Required (string)
- `text` OR `media_urls`: At least one must be present
- `Authorization` header must match configured API key

**Success Response (200 OK):**
```json
{
  "data": {
    "id": "uuid-here",
    "record_type": "message",
    "from": "+1234567890",
    "to": "+0987654321",
    "text": "Hello, world!",
    "media_urls": [],
    "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "direction": "outbound",
    "status": "queued",
    "created_at": "2024-01-01T12:00:00Z",
    "updated_at": "2024-01-01T12:00:00Z"
  }
}
```

**Optional Request Fields:**
- `webhook_url` (string) - Custom webhook URL for status updates
- `webhook_failover_url` (string) - Fallback webhook URL
- `use_profile_webhooks` (boolean) - Use messaging profile webhook settings

**Error Response (422 Unprocessable Entity):**
```json
{
  "errors": [
    {
      "code": "10005",
      "title": "Invalid parameter",
      "detail": "The 'to' parameter is required."
    }
  ]
}
```

**Unauthorized Response (401):**
```json
{
  "errors": [
    {
      "code": "10001",
      "title": "Unauthorized",
      "detail": "Invalid API key."
    }
  ]
}
```

### POST /v2/webhooks/messages

Receive inbound messages (webhook endpoint). Supports both Telnyx webhook format and simple JSON.

**Simple Format:**
```json
{
  "from": "+0987654321",
  "to": "+1234567890",
  "text": "Reply message"
}
```

**Telnyx Webhook Format:**
```json
{
  "data": {
    "event_type": "message.received",
    "payload": {
      "id": "message-id",
      "from": "+0987654321",
      "to": "+1234567890",
      "text": "Reply message",
      "direction": "inbound"
    }
  }
}
```

## Status Callbacks (Outbound Webhooks)

When you send a message with a `webhook_url` in the request, SmsSink will automatically send status callbacks to that URL, simulating Telnyx's delivery notifications.

**Status Sequence:**
1. `message.sent` - Sent ~500ms after message creation
2. `message.delivered` - Sent ~1.5s after message creation

**Example Request with Webhook:**
```bash
curl -X POST http://localhost:23456/v2/messages \
  -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "+1234567890",
    "to": "+0987654321",
    "text": "Hello!",
    "messaging_profile_id": "profile-123",
    "webhook_url": "https://your-app.com/webhooks/telnyx",
    "webhook_failover_url": "https://your-app.com/webhooks/telnyx-backup"
  }'
```

**Webhook Payload Format:**
```json
{
  "data": {
    "event_type": "message.delivered",
    "id": "event-uuid",
    "occurred_at": "2024-01-01T12:00:00Z",
    "record_type": "event",
    "payload": {
      "id": "message-uuid",
      "direction": "outbound",
      "messaging_profile_id": "profile-123",
      "from": {
        "phone_number": "+1234567890",
        "carrier": "SmsSink Mock Carrier",
        "line_type": "Wireless"
      },
      "to": [{
        "phone_number": "+0987654321",
        "status": "delivered",
        "carrier": "SmsSink Mock Carrier",
        "line_type": "Wireless"
      }],
      "text": "Hello!",
      "type": "SMS",
      "status": "delivered",
      "sent_at": "2024-01-01T12:00:00Z",
      "completed_at": "2024-01-01T12:00:01Z"
    }
  }
}
```

**Webhook Headers:**
- `Content-Type: application/json`
- `User-Agent: SmsSink/1.0`
- `telnyx-timestamp: <RFC3339 timestamp>`
- `telnyx-signature-ed25519: mock-signature` (placeholder - not cryptographically valid)

**Failover Behavior:**
If the primary `webhook_url` returns a non-2xx status, SmsSink will automatically try the `webhook_failover_url` if provided.

## Web UI Endpoints

### GET /

Serves the embedded HTML dashboard with message inspector and inbound simulation form.

### GET /api/messages

Returns JSON array of all messages (newest first).

**Response:**
```json
[
  {
    "id": "uuid",
    "created_at": "2024-01-01T12:00:00Z",
    "sender": "+1234567890",
    "recipient": "+0987654321",
    "content": "Hello!",
    "media_urls": "[]",
    "direction": "outbound"
  }
]
```

### DELETE /api/messages

Clears all messages from the database.

### POST /api/messages/inbound

Simulate an inbound message (for testing).

**Request:**
```json
{
  "from": "+0987654321",
  "to": "+1234567890",
  "text": "Test inbound message"
}
```

### GET /api/credentials

Get current API credentials.

**Response:**
```json
{
  "api_key": "test-token",
  "updated_at": "2024-01-01T12:00:00Z"
}
```

### POST /api/credentials

Update API credentials.

**Request:**
```json
{
  "api_key": "new-api-key"
}
```

### GET /credentials

Serves the credentials management page.

## Example Usage

### Send an outbound message:

```bash
curl -X POST http://localhost:23456/v2/messages \
  -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "+1234567890",
    "to": "+0987654321",
    "text": "Hello from SmsSink!",
    "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }'
```

### Receive an inbound message (webhook):

```bash
curl -X POST http://localhost:23456/v2/webhooks/messages \
  -H "Content-Type: application/json" \
  -d '{
    "from": "+0987654321",
    "to": "+1234567890",
    "text": "Reply message"
  }'
```

### Test validation (missing 'to' field):

```bash
curl -X POST http://localhost:23456/v2/messages \
  -H "Authorization: Bearer test-token" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "+1234567890",
    "text": "This should fail",
    "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }'
```

Expected response: `422 Unprocessable Entity` with error details.

### Test invalid API key:

```bash
curl -X POST http://localhost:23456/v2/messages \
  -H "Authorization: Bearer wrong-key" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "+1234567890",
    "to": "+0987654321",
    "text": "This should fail",
    "messaging_profile_id": "400017d2-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  }'
```

Expected response: `401 Unauthorized` with error details.

## Web UI Features

### Message Inspector Dashboard

- **Real-time Updates**: Auto-refreshes every 10 seconds (configurable)
- **Direction Filtering**: Visual distinction between inbound (blue) and outbound (green) messages
- **Message Details**: View timestamp, direction, sender, recipient, content, and media URLs
- **Clear Messages**: Delete all messages with one click
- **Inbound Simulation**: Form to create test inbound messages

### Credentials Management

- **View Current API Key**: See the currently configured API key
- **Update API Key**: Change the API key used for authentication
- **Copy to Clipboard**: One-click copy of API key
- **Dynamic Examples**: Code examples update automatically with your API key

## Database Schema

The SQLite database contains two tables:

### Messages Table

```sql
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    created_at DATETIME NOT NULL,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    content TEXT,
    media_urls TEXT,
    messaging_profile_id TEXT,
    direction TEXT NOT NULL
);
```

### Credentials Table

```sql
CREATE TABLE credentials (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    api_key TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);
```

## Architecture

```
SmsSink/
├── main.go                    # Server setup, routing, graceful shutdown
├── internal/
│   ├── validator/             # Strict validation logic matching Telnyx API rules
│   │   └── validator.go
│   ├── database/              # SQLite database operations
│   │   └── db.go
│   ├── server/                # HTTP handlers for API and UI endpoints
│   │   └── handlers.go
│   └── ui/
│       └── assets/            # Embedded HTML/CSS/JS for web dashboard
│           ├── index.html
│           └── credentials.html
└── go.mod                     # Go module dependencies
```

## Message Flow

### Outbound Messages

1. Client sends POST request to `/v2/messages` with API key
2. Server validates API key and request payload
3. If valid, message is stored with `direction: "outbound"`
4. Success response returned to client

### Inbound Messages

1. Webhook sends POST request to `/v2/webhooks/messages`
2. Server parses webhook payload (Telnyx format or simple JSON)
3. Message is stored with `direction: "inbound"`
4. Success response returned to webhook sender

## Configuration

### Default Settings

- **API Server Port**: 23456
- **UI Server Port**: 23457
- **Database File**: `smssink.db` (created in current directory)
- **Default API Key**: `test-token`

### Environment Variables

Currently, all configuration is hardcoded. Future versions may support environment variables for:
- Port numbers
- Database path
- Default API key

## Graceful Shutdown

The server supports graceful shutdown on SIGINT or SIGTERM signals, ensuring all connections are properly closed before exit.

## Development

### Prerequisites

- Go 1.22 or later
- No CGO required (pure Go SQLite driver)

### Building from Source

```bash
git clone <repository>
cd smstrap
go mod download
go build -o SmsSink .
```

### Running Tests

```bash
go test ./...
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]

## Support

For issues, questions, or contributions, please open an issue on the repository.
