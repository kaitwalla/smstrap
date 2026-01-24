package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID                 string    `json:"id"`
	CreatedAt          time.Time `json:"created_at"`
	Sender             string    `json:"sender"`
	Recipient          string    `json:"recipient"`
	Content            string    `json:"content"`
	MediaURLs          string    `json:"media_urls"` // Stored as JSON string
	MessagingProfileID string    `json:"messaging_profile_id"`
	Direction          string    `json:"direction"`
}

// LogEntry represents an application log entry
type LogEntry struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Level     string    `json:"level"`     // info, warning, error
	Category  string    `json:"category"`  // message, webhook, auth, system
	Message   string    `json:"message"`
	Details   string    `json:"details"`   // JSON string with extra context
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
		messaging_profile_id TEXT,
		direction TEXT NOT NULL
	);
	`

	_, err = DB.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Add messaging_profile_id column if it doesn't exist (migration for existing databases)
	// SQLite doesn't support IF NOT EXISTS for ALTER TABLE ADD COLUMN, so we check first
	var columnExists int
	err = DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='messaging_profile_id'").Scan(&columnExists)
	if err == nil && columnExists == 0 {
		_, err = DB.Exec("ALTER TABLE messages ADD COLUMN messaging_profile_id TEXT")
		if err != nil {
			// Ignore error if column already exists (race condition)
			_ = err
		}
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

	// Create logs table for application logging
	createLogsSQL := `
	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME NOT NULL,
		level TEXT NOT NULL,
		category TEXT NOT NULL,
		message TEXT NOT NULL,
		details TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_logs_created_at ON logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
	CREATE INDEX IF NOT EXISTS idx_logs_category ON logs(category);
	`

	_, err = DB.Exec(createLogsSQL)
	if err != nil {
		return fmt.Errorf("failed to create logs table: %w", err)
	}

	// Clean up logs older than 7 days on startup
	if err := CleanupOldLogs(7); err != nil {
		// Log the error but don't fail initialization
		fmt.Printf("Warning: failed to cleanup old logs: %v\n", err)
	}

	return nil
}

// InsertMessage inserts a new message into the database
func InsertMessage(id, sender, recipient, content string, mediaURLs []string, messagingProfileID string, direction string) error {
	mediaURLsJSON := "[]"
	if len(mediaURLs) > 0 {
		jsonBytes, err := json.Marshal(mediaURLs)
		if err != nil {
			return fmt.Errorf("failed to marshal media_urls: %w", err)
		}
		mediaURLsJSON = string(jsonBytes)
	}

	query := `
		INSERT INTO messages (id, created_at, sender, recipient, content, media_urls, messaging_profile_id, direction)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := DB.Exec(query, id, time.Now().UTC(), sender, recipient, content, mediaURLsJSON, messagingProfileID, direction)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// GetAllMessages retrieves all messages from the database, ordered by created_at DESC
func GetAllMessages() ([]Message, error) {
	query := `
		SELECT id, created_at, sender, recipient, content, media_urls, messaging_profile_id, direction
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
		err := rows.Scan(&msg.ID, &msg.CreatedAt, &msg.Sender, &msg.Recipient, &msg.Content, &msg.MediaURLs, &msg.MessagingProfileID, &msg.Direction)
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
	
	// Extract token from auth header - support multiple formats
	token := authHeader
	
	// Handle "Bearer <token>" format
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}
	
	// Handle "Basic <token>" format (some SDKs use this)
	if len(authHeader) > 6 && authHeader[:6] == "Basic " {
		token = authHeader[6:]
	}
	
	// Compare token with stored API key
	return token == cred.APIKey
}

// GetExpectedToken returns the stored API key for debugging purposes
func GetExpectedToken() string {
	cred, err := GetCredential()
	if err != nil {
		return ""
	}
	return cred.APIKey
}

// CloseDB closes the database connection
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// InsertLog adds a new log entry to the database
func InsertLog(level, category, message string, details map[string]interface{}) error {
	// Gracefully handle case where DB is not initialized (e.g., in tests)
	if DB == nil {
		return nil
	}

	detailsJSON := ""
	if details != nil {
		jsonBytes, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("failed to marshal log details: %w", err)
		}
		detailsJSON = string(jsonBytes)
	}

	query := `
		INSERT INTO logs (created_at, level, category, message, details)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := DB.Exec(query, time.Now().UTC(), level, category, message, detailsJSON)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}

	return nil
}

// Log is a convenience function for logging info level messages
func Log(category, message string, details map[string]interface{}) {
	_ = InsertLog("info", category, message, details)
}

// LogError is a convenience function for logging error level messages
func LogError(category, message string, details map[string]interface{}) {
	_ = InsertLog("error", category, message, details)
}

// LogWarning is a convenience function for logging warning level messages
func LogWarning(category, message string, details map[string]interface{}) {
	_ = InsertLog("warning", category, message, details)
}

// GetLogs retrieves log entries, optionally filtered by level and category
func GetLogs(level, category string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, created_at, level, category, message, details
		FROM logs
		WHERE (? = '' OR level = ?)
		  AND (? = '' OR category = ?)
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := DB.Query(query, level, level, category, category, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	logs := []LogEntry{}
	for rows.Next() {
		var log LogEntry
		var details sql.NullString
		err := rows.Scan(&log.ID, &log.CreatedAt, &log.Level, &log.Category, &log.Message, &details)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}
		if details.Valid {
			log.Details = details.String
		}
		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	return logs, nil
}

// CleanupOldLogs removes log entries older than the specified number of days
func CleanupOldLogs(days int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	
	result, err := DB.Exec("DELETE FROM logs WHERE created_at < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old logs: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		fmt.Printf("Cleaned up %d log entries older than %d days\n", affected, days)
	}

	return nil
}

// ClearAllLogs removes all log entries
func ClearAllLogs() error {
	_, err := DB.Exec("DELETE FROM logs")
	if err != nil {
		return fmt.Errorf("failed to clear logs: %w", err)
	}
	return nil
}
