package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"`
	Content   string    `json:"content"`
	MediaURLs string    `json:"media_urls"` // Stored as JSON string
	Direction string    `json:"direction"`
}

var DB *sql.DB

// InitDB initializes the SQLite database and creates the messages table
func InitDB(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create messages table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL,
		content TEXT,
		media_urls TEXT,
		direction TEXT NOT NULL
	);
	`

	_, err = DB.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Create credentials table (single row for API key)
	createCredentialsSQL := `
	CREATE TABLE IF NOT EXISTS credentials (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		api_key TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`

	_, err = DB.Exec(createCredentialsSQL)
	if err != nil {
		return fmt.Errorf("failed to create credentials table: %w", err)
	}

	// Initialize with default API key if none exists
	var count int
	err = DB.QueryRow("SELECT COUNT(*) FROM credentials").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check credentials: %w", err)
	}

	if count == 0 {
		defaultKey := "test-token"
		_, err = DB.Exec("INSERT INTO credentials (id, api_key, updated_at) VALUES (1, ?, ?)", defaultKey, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to initialize default credentials: %w", err)
		}
	}

	return nil
}

// InsertMessage inserts a new message into the database
func InsertMessage(id, sender, recipient, content string, mediaURLs []string, direction string) error {
	mediaURLsJSON := "[]"
	if len(mediaURLs) > 0 {
		jsonBytes, err := json.Marshal(mediaURLs)
		if err != nil {
			return fmt.Errorf("failed to marshal media_urls: %w", err)
		}
		mediaURLsJSON = string(jsonBytes)
	}

	query := `
		INSERT INTO messages (id, created_at, sender, recipient, content, media_urls, direction)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := DB.Exec(query, id, time.Now().UTC(), sender, recipient, content, mediaURLsJSON, direction)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// GetAllMessages retrieves all messages from the database, ordered by created_at DESC
func GetAllMessages() ([]Message, error) {
	query := `
		SELECT id, created_at, sender, recipient, content, media_urls, direction
		FROM messages
		ORDER BY created_at DESC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	messages := []Message{} // Initialize as empty slice, not nil, so JSON encodes as [] not null
	for rows.Next() {
		var msg Message
		err := rows.Scan(&msg.ID, &msg.CreatedAt, &msg.Sender, &msg.Recipient, &msg.Content, &msg.MediaURLs, &msg.Direction)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// ClearAllMessages truncates the messages table
func ClearAllMessages() error {
	_, err := DB.Exec("DELETE FROM messages")
	if err != nil {
		return fmt.Errorf("failed to clear messages: %w", err)
	}
	return nil
}

// Credential represents stored API credentials
type Credential struct {
	APIKey    string    `json:"api_key"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetCredential retrieves the stored API key
func GetCredential() (*Credential, error) {
	var cred Credential
	err := DB.QueryRow("SELECT api_key, updated_at FROM credentials WHERE id = 1").Scan(&cred.APIKey, &cred.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return default if no credentials exist
			return &Credential{
				APIKey:    "test-token",
				UpdatedAt: time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}
	return &cred, nil
}

// SetCredential updates the stored API key
func SetCredential(apiKey string) error {
	// Use INSERT OR REPLACE to handle both insert and update (SQLite-specific syntax)
	query := `
		INSERT OR REPLACE INTO credentials (id, api_key, updated_at)
		VALUES (1, ?, ?)
	`
	_, err := DB.Exec(query, apiKey, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to set credential: %w", err)
	}
	return nil
}

// ValidateCredential checks if the provided auth header matches the stored credential
func ValidateCredential(authHeader string) bool {
	cred, err := GetCredential()
	if err != nil {
		return false
	}
	
	// Support both "Bearer <token>" and just "<token>" formats
	if authHeader == cred.APIKey {
		return true
	}
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:] == cred.APIKey
	}
	return false
}

// CloseDB closes the database connection
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
