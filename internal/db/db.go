// Package db provides SQLite-based persistent storage for microclaw.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection for microclaw storage.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	d := &DB{db: db}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// migrate creates all required tables if they don't exist.
func (d *DB) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id     INTEGER NOT NULL,
			chat_channel TEXT NOT NULL,
			sender_name TEXT NOT NULL,
			content     TEXT NOT NULL,
			is_from_bot INTEGER NOT NULL DEFAULT 0,
			timestamp   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, chat_channel)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			chat_id      INTEGER NOT NULL,
			channel      TEXT NOT NULL,
			session_data TEXT NOT NULL DEFAULT '{}',
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (chat_id, channel)
		)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id        INTEGER,
			chat_channel   TEXT,
			category       TEXT NOT NULL DEFAULT 'general',
			content        TEXT NOT NULL,
			embedding_model TEXT,
			created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS scheduled_tasks (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id      INTEGER NOT NULL,
			chat_channel TEXT NOT NULL,
			name         TEXT NOT NULL,
			description  TEXT NOT NULL DEFAULT '',
			cron_expr    TEXT,
			one_time_at  DATETIME,
			last_run     DATETIME,
			next_run     DATETIME,
			status       TEXT NOT NULL DEFAULT 'active',
			run_count    INTEGER NOT NULL DEFAULT 0,
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS task_run_history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id    INTEGER NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			output     TEXT,
			success    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS todos (
			chat_id      INTEGER NOT NULL,
			chat_channel TEXT NOT NULL,
			content      TEXT NOT NULL DEFAULT '',
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (chat_id, chat_channel)
		)`,
		`CREATE TABLE IF NOT EXISTS web_auth (
			id           INTEGER PRIMARY KEY CHECK (id = 1),
			password_hash TEXT NOT NULL,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, stmt := range statements {
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed (%q): %w", stmt[:min(60, len(stmt))], err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Message represents a stored chat message.
type Message struct {
	ID          int64
	ChatID      int64
	ChatChannel string
	SenderName  string
	Content     string
	IsFromBot   bool
	Timestamp   time.Time
}

// SaveMessage persists a message to the database.
func (d *DB) SaveMessage(m *Message) error {
	_, err := d.db.Exec(
		`INSERT INTO messages (chat_id, chat_channel, sender_name, content, is_from_bot, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.ChatID, m.ChatChannel, m.SenderName, m.Content, boolToInt(m.IsFromBot), m.Timestamp,
	)
	return err
}

// GetMessages retrieves recent messages for a chat.
func (d *DB) GetMessages(chatID int64, channel string, limit int) ([]*Message, error) {
	rows, err := d.db.Query(
		`SELECT id, chat_id, chat_channel, sender_name, content, is_from_bot, timestamp
		 FROM messages WHERE chat_id = ? AND chat_channel = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		chatID, channel, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		var m Message
		var isBot int
		if err := rows.Scan(&m.ID, &m.ChatID, &m.ChatChannel, &m.SenderName, &m.Content, &isBot, &m.Timestamp); err != nil {
			return nil, err
		}
		m.IsFromBot = isBot != 0
		msgs = append(msgs, &m)
	}
	// Reverse to get chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, rows.Err()
}

// GetSessionData returns session JSON data for a chat, empty string if not found.
func (d *DB) GetSessionData(chatID int64, channel string) (string, error) {
	var data string
	err := d.db.QueryRow(
		`SELECT session_data FROM sessions WHERE chat_id = ? AND channel = ?`,
		chatID, channel,
	).Scan(&data)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return data, err
}

// SaveSessionData upserts the session JSON data.
func (d *DB) SaveSessionData(chatID int64, channel, data string) error {
	_, err := d.db.Exec(
		`INSERT INTO sessions (chat_id, channel, session_data, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(chat_id, channel) DO UPDATE SET
		   session_data = excluded.session_data,
		   updated_at = excluded.updated_at`,
		chatID, channel, data,
	)
	return err
}

// DeleteSession removes a session.
func (d *DB) DeleteSession(chatID int64, channel string) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE chat_id = ? AND channel = ?`, chatID, channel)
	return err
}

// DeleteMessages removes all messages for a chat.
func (d *DB) DeleteMessages(chatID int64, channel string) error {
	_, err := d.db.Exec(`DELETE FROM messages WHERE chat_id = ? AND chat_channel = ?`, chatID, channel)
	return err
}

// GetTodo retrieves the todo list for a chat.
func (d *DB) GetTodo(chatID int64, channel string) (string, error) {
	var content string
	err := d.db.QueryRow(
		`SELECT content FROM todos WHERE chat_id = ? AND chat_channel = ?`,
		chatID, channel,
	).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

// SaveTodo upserts the todo list.
func (d *DB) SaveTodo(chatID int64, channel, content string) error {
	_, err := d.db.Exec(
		`INSERT INTO todos (chat_id, chat_channel, content, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(chat_id, chat_channel) DO UPDATE SET
		   content = excluded.content,
		   updated_at = excluded.updated_at`,
		chatID, channel, content,
	)
	return err
}

// ScheduledTask represents a scheduled task.
type ScheduledTask struct {
	ID          int64
	ChatID      int64
	ChatChannel string
	Name        string
	Description string
	CronExpr    string
	OneTimeAt   *time.Time
	LastRun     *time.Time
	NextRun     *time.Time
	Status      string
	RunCount    int
	CreatedAt   time.Time
}

// SaveScheduledTask creates a new scheduled task.
func (d *DB) SaveScheduledTask(t *ScheduledTask) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO scheduled_tasks (chat_id, chat_channel, name, description, cron_expr, one_time_at, next_run, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'active')`,
		t.ChatID, t.ChatChannel, t.Name, t.Description, nilStr(t.CronExpr), t.OneTimeAt, t.NextRun,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetScheduledTasks retrieves active tasks for a chat.
func (d *DB) GetScheduledTasks(chatID int64, channel string) ([]*ScheduledTask, error) {
	rows, err := d.db.Query(
		`SELECT id, chat_id, chat_channel, name, description, COALESCE(cron_expr,''), status, run_count, created_at
		 FROM scheduled_tasks WHERE chat_id = ? AND chat_channel = ? AND status = 'active'`,
		chatID, channel,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		if err := rows.Scan(&t.ID, &t.ChatID, &t.ChatChannel, &t.Name, &t.Description, &t.CronExpr, &t.Status, &t.RunCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

// DeleteScheduledTask removes a task by ID.
func (d *DB) DeleteScheduledTask(id int64) error {
	_, err := d.db.Exec(`UPDATE scheduled_tasks SET status = 'deleted' WHERE id = ?`, id)
	return err
}

// RecordTaskRun appends a run history entry.
func (d *DB) RecordTaskRun(taskID int64, startedAt, finishedAt time.Time, output string, success bool) error {
	_, err := d.db.Exec(
		`INSERT INTO task_run_history (task_id, started_at, finished_at, output, success) VALUES (?, ?, ?, ?, ?)`,
		taskID, startedAt, finishedAt, output, boolToInt(success),
	)
	return err
}

// SetWebPassword stores a bcrypt-hashed web UI password.
func (d *DB) SetWebPassword(hash string) error {
	_, err := d.db.Exec(
		`INSERT INTO web_auth (id, password_hash, updated_at) VALUES (1, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at`,
		hash,
	)
	return err
}

// GetWebPasswordHash retrieves the stored password hash, empty if not set.
func (d *DB) GetWebPasswordHash() (string, error) {
	var hash string
	err := d.db.QueryRow(`SELECT password_hash FROM web_auth WHERE id = 1`).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

// GetAllScheduledTasks returns all active tasks across all chats.
func (d *DB) GetAllScheduledTasks() ([]*ScheduledTask, error) {
	rows, err := d.db.Query(
		`SELECT id, chat_id, chat_channel, name, description, COALESCE(cron_expr,''), status, run_count, created_at
		 FROM scheduled_tasks WHERE status = 'active'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*ScheduledTask
	for rows.Next() {
		var t ScheduledTask
		if err := rows.Scan(&t.ID, &t.ChatID, &t.ChatChannel, &t.Name, &t.Description, &t.CronExpr, &t.Status, &t.RunCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, &t)
	}
	return tasks, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nilStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
