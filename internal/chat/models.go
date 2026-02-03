package chat

import "time"

type Session struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"-"`
	SessionID string    `gorm:"type:varchar(26);uniqueIndex;not null" json:"session_id"`
	UserID    uint64    `gorm:"index;not null" json:"-"`
	Provider  string    `gorm:"type:varchar(32);not null" json:"provider"`
	Model     string    `gorm:"type:varchar(64);not null" json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Session) TableName() string { return "chat_sessions" }

type Message struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID      string    `gorm:"type:varchar(26);not null;index:idx_chat_msg_user_session_id,priority:2;index:uniq_chat_msg_idempo,unique,priority:2" json:"session_id"`
	UserID         uint64    `gorm:"not null;index:idx_chat_msg_user_session_id,priority:1;index:uniq_chat_msg_idempo,unique,priority:1" json:"-"`
	Role           string    `gorm:"type:varchar(16);index;not null" json:"role"`
	Content        string    `gorm:"type:text;not null" json:"content"`
	IdempotencyKey *string   `gorm:"type:varchar(128);index:uniq_chat_msg_idempo,unique,priority:3" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

func (Message) TableName() string { return "chat_messages" }
