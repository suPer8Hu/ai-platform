package chat

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) CreateSession(ctx context.Context, s *Session) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *Repo) GetSessionBySessionID(ctx context.Context, sessionID string) (*Session, error) {
	var s Session
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repo) InsertMessage(ctx context.Context, m *Message) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// ListMessages returns messages in DESC id order (newest -> oldest).
func (r *Repo) ListMessages(ctx context.Context, userID uint64, sessionID string, limit int, beforeID uint64) ([]Message, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("id DESC").
		Limit(limit)

	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}

	var msgs []Message
	if err := q.Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// ListRecentMessagesDesc returns the most recent messages in DESC id order (newest -> oldest).
func (r *Repo) ListRecentMessagesDesc(ctx context.Context, userID uint64, sessionID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 20
	}
	var msgs []Message
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("id DESC").
		Limit(limit).
		Find(&msgs).Error; err != nil {
		return nil, err
	}
	return msgs, nil
}

// Job CRUD
func (r *Repo) CreateJob(ctx context.Context, job *Job) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *Repo) GetJobByID(ctx context.Context, id string) (*Job, error) {
	var j Job
	if err := r.db.WithContext(ctx).First(&j, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repo) UpdateJobStatusRunning(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status = ?", id, JobQueued).
		Update("status", JobRunning).Error
}

func (r *Repo) MarkJobSucceeded(ctx context.Context, id string, assistantMsgID uint64) error {
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":            JobSucceeded,
			"result_message_id": assistantMsgID,
			"error":             nil,
		}).Error
}

func (r *Repo) MarkJobFailed(ctx context.Context, id string, errMsg string) error {
	return r.db.WithContext(ctx).Model(&Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":            JobFailed,
			"error":             errMsg,
			"result_message_id": nil,
		}).Error
}

func (r *Repo) GetJobByUserAndIdempotencyKey(ctx context.Context, userID uint64, key string) (*Job, error) {
	var job Job
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ?", userID, key).
		First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// CreateJobOrGetExisting tries to create a job, but if (user_id, idempotency_key) already exists,
// it returns the existing job instead.

func (r *Repo) CreateJobOrGetExisting(ctx context.Context, job *Job) (*Job, bool, error) {
	if job.IdempotencyKey == nil || *job.IdempotencyKey == "" {
		// No key provided -> old behavior
		job.IdempotencyKey = nil
		if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
			return nil, false, err
		}
		return job, true, nil
	}

	err := r.db.WithContext(ctx).Create(job).Error
	if err == nil {
		return job, true, nil
	}

	existing, getErr := r.GetJobByUserAndIdempotencyKey(ctx, job.UserID, *job.IdempotencyKey)
	if getErr == nil {
		return existing, false, nil
	}

	if errors.Is(getErr, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	return nil, false, getErr
}
