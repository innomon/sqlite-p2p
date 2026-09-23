package whatsadk

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// StoreBackend defines the storage contract required by whatsadk.
type StoreBackend interface {
	Close() error
	EnqueueCommand(ctx context.Context, cmd string, payload interface{}) (int64, error)
	UpdateCommandStatus(ctx context.Context, id int64, status string, result interface{}) error
	PollPendingCommands(ctx context.Context) ([]Command, error)
	WaitForCommand(ctx context.Context, id int64, timeout time.Duration) (*Command, error)
	PutFile(ctx context.Context, path string, metadata interface{}, content []byte, timestamp time.Time) error
	IsBlacklisted(ctx context.Context, phone string) (bool, error)
	AddBlacklist(ctx context.Context, phone, reason string) error
	RemoveBlacklist(ctx context.Context, phone string) error
	ListBlacklist(ctx context.Context) ([]BlacklistedNumber, error)
	ListContacts(ctx context.Context, query string) ([]Contact, error)
	GetFilesysLogs(ctx context.Context, phone string, limit int) ([]FileEntry, error)
	GetLatestGlobalMessages(ctx context.Context, limit int) ([]FileEntry, error)
	QueryFilesys(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	GetFile(ctx context.Context, path string) (*FileEntry, error)
	DeleteFile(ctx context.Context, path string) error
	ListFiles(ctx context.Context, prefix string, limit int) ([]FileEntry, error)
	GetAllContacts(ctx context.Context) ([]Contact, error)
	PutContact(ctx context.Context, contact Contact) error
	GetAllCommands(ctx context.Context) ([]Command, error)
	PutCommand(ctx context.Context, cmd Command) error
	GetAllFiles(ctx context.Context) ([]FileEntry, error)
	ResetSequence(ctx context.Context) error
}

// Command models an asynchronous command execution record in whatsadk.
type Command struct {
	ID        int64           `json:"id"`
	Command   string          `json:"command"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Contact models a WhatsApp contact entity in whatsadk.
type Contact struct {
	OurJID       string `json:"our_jid"`
	TheirJID     string `json:"their_jid"`
	FullName     string `json:"full_name"`
	ShortName    string `json:"short_name"`
	PushName     string `json:"push_name"`
	BusinessName string `json:"business_name"`
}

// FileEntry models a virtual filesystem object or message in whatsadk.
type FileEntry struct {
	Path      string         `json:"path"`
	Metadata  sql.NullString `json:"metadata"`
	Content   []byte         `json:"content,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// BlacklistedNumber models a phone number prohibited from interacting.
type BlacklistedNumber struct {
	Phone     string    `json:"phone"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
