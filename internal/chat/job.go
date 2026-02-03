package chat

import "time"

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID string `gorm:"primaryKey;size:26"` // ULID length

	UserID    uint64 `gorm:"index;not null"`
	SessionID string `gorm:"size:26;index;not null"`

	Prompt string `gorm:"type:text;not null"`

	IdempotencyKey *string `gorm:"type:varchar(128);index:uniq_user_idempo,unique" json:"idempotency_key"`

	Status JobStatus `gorm:"type:varchar(16);index;not null"`

	// Filled when succeeded
	ResultMessageID *uint64 `gorm:"index"`

	// Filled when failed
	Error *string `gorm:"type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time
}
