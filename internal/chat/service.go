package chat

import (
	"context"
	"errors"
	"strings"

	"github.com/suPer8Hu/ai-platform/internal/ai"
	"gorm.io/gorm"
)

type Service struct {
	repo              *Repo
	registry          *ai.Registry
	contextWindowSize int
}

func NewService(repo *Repo, registry *ai.Registry, contextWindowSize int) *Service {
	if contextWindowSize <= 0 || contextWindowSize > 100 {
		contextWindowSize = 20
	}
	return &Service{repo: repo, registry: registry, contextWindowSize: contextWindowSize}
}

const (
	defaultProvider = "ollama"
	defaultModel    = "llama3:latest"
)

func (s *Service) CreateSession(ctx context.Context, userID uint64, provider, model string) (*Session, error) {
	if provider == "" {
		provider = defaultProvider
	}
	if model == "" {
		model = defaultModel
	}

	sid, err := NewSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		SessionID: sid,
		UserID:    userID,
		Provider:  provider,
		Model:     model,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Service) providerForSession(ctx context.Context, sess *Session) (ai.Provider, error) {
	p := sess.Provider
	m := sess.Model
	if p == "" {
		p = defaultProvider
	}
	if m == "" {
		m = defaultModel
	}
	return s.registry.Get(ctx, p, m)
}

func (s *Service) SendMessage(ctx context.Context, userID uint64, sessionID string, content string) (reply string, assistantMsgID uint64, err error) {
	// 1) verify session ownership
	session, err := s.repo.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, err
		}
		return "", 0, err
	}
	if session.UserID != userID {
		return "", 0, gorm.ErrRecordNotFound
	}

	//  pick provider/model for this session
	provider, err := s.providerForSession(ctx, session)
	if err != nil {
		return "", 0, err
	}

	// 2) store user message (strong consistency)
	userMsg := &Message{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   content,
	}
	if err := s.repo.InsertMessage(ctx, userMsg); err != nil {
		return "", 0, err
	}

	// 3) build provider messages from recent DB history
	recentDesc, err := s.repo.ListRecentMessagesDesc(ctx, userID, sessionID, s.contextWindowSize)
	if err != nil {
		return "", 0, err
	}

	// reverse to ASC (oldest -> newest)
	providerMsgs := make([]ai.Message, 0, len(recentDesc))
	for i := len(recentDesc) - 1; i >= 0; i-- {
		m := recentDesc[i]
		providerMsgs = append(providerMsgs, ai.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// 4) call provider
	reply, err = provider.Chat(ctx, providerMsgs)
	if err != nil {
		return "", 0, err
	}

	// 5) store assistant message (strong consistency)
	assistantMsg := &Message{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "assistant",
		Content:   reply,
	}
	if err := s.repo.InsertMessage(ctx, assistantMsg); err != nil {
		return "", 0, err
	}

	return reply, assistantMsg.ID, nil
}

func (s *Service) ListMessages(ctx context.Context, userID uint64, sessionID string, limit int, beforeID uint64) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListMessages(ctx, userID, sessionID, limit, beforeID)
}

// SendMessageStream stores the user message immediately, streams assistant chunks,
// and finally stores the assistant message after streaming completes.
func (s *Service) SendMessageStream(ctx context.Context, userID uint64, sessionID string, content string) (chunks <-chan string, done <-chan struct{}, assistantMsgID <-chan uint64, errs <-chan error) {
	outChunks := make(chan string, 16)
	outDone := make(chan struct{})
	outMsgID := make(chan uint64, 1)
	outErrs := make(chan error, 1)

	go func() {
		defer close(outChunks)
		defer close(outDone)
		defer close(outMsgID)
		defer close(outErrs)

		// 1) session ownership check
		sess, err := s.repo.GetSessionBySessionID(ctx, sessionID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				outErrs <- gorm.ErrRecordNotFound
				return
			}
			outErrs <- err
			return
		}
		if sess.UserID != userID {
			outErrs <- gorm.ErrRecordNotFound
			return
		}

		// pick provider/model for this session
		provider, err := s.providerForSession(ctx, sess)
		if err != nil {
			outErrs <- err
			return
		}

		// 2) insert user message
		userMsg := &Message{
			SessionID: sessionID,
			UserID:    userID,
			Role:      "user",
			Content:   content,
		}
		if err := s.repo.InsertMessage(ctx, userMsg); err != nil {
			outErrs <- err
			return
		}

		// 3) load recent messages, build provider context (ASC)
		recentDesc, err := s.repo.ListRecentMessagesDesc(ctx, userID, sessionID, s.contextWindowSize)
		if err != nil {
			outErrs <- err
			return
		}
		providerMsgs := make([]ai.Message, 0, len(recentDesc))
		for i := len(recentDesc) - 1; i >= 0; i-- {
			m := recentDesc[i]
			providerMsgs = append(providerMsgs, ai.Message{Role: m.Role, Content: m.Content})
		}

		sp, ok := provider.(ai.StreamProvider)
		if !ok {
			outErrs <- errors.New("provider does not support streaming")
			return
		}

		// 4) stream from provider
		pChunks, pErrs := sp.StreamChat(ctx, providerMsgs)

		var b strings.Builder
		for c := range pChunks {
			b.WriteString(c)
			outChunks <- c
		}

		// provider error (if any)
		select {
		case err := <-pErrs:
			if err != nil {
				outErrs <- err
				return
			}
		default:
			// no error sent
		}

		reply := b.String()

		// 5) insert assistant message at the end
		assistantMsg := &Message{
			SessionID: sessionID,
			UserID:    userID,
			Role:      "assistant",
			Content:   reply,
		}
		if err := s.repo.InsertMessage(ctx, assistantMsg); err != nil {
			outErrs <- err
			return
		}

		outMsgID <- assistantMsg.ID
	}()

	return outChunks, outDone, outMsgID, outErrs
}

func (s *Service) ValidateSessionOwner(ctx context.Context, userID uint64, sessionID string) error {
	sess, err := s.repo.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	if sess.UserID != userID {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) InsertUserMessage(ctx context.Context, userID uint64, sessionID string, content string) error {
	// session ownership check
	if err := s.ValidateSessionOwner(ctx, userID, sessionID); err != nil {
		return err
	}
	return s.repo.InsertMessage(ctx, &Message{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   content,
	})
}

func (s *Service) CreateJob(ctx context.Context, job *Job) error {
	return s.repo.CreateJob(ctx, job)
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*Job, error) {
	return s.repo.GetJobByID(ctx, jobID)
}

func (s *Service) GenerateAssistantReplyAndInsert(ctx context.Context, userID uint64, sessionID string) (string, uint64, error) {
	// session ownership check + get session for provider routing
	sess, err := s.repo.GetSessionBySessionID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, gorm.ErrRecordNotFound
		}
		return "", 0, err
	}
	if sess.UserID != userID {
		return "", 0, gorm.ErrRecordNotFound
	}

	provider, err := s.providerForSession(ctx, sess)
	if err != nil {
		return "", 0, err
	}

	recentDesc, err := s.repo.ListRecentMessagesDesc(ctx, userID, sessionID, s.contextWindowSize)
	if err != nil {
		return "", 0, err
	}

	// provider expects ASC
	providerMsgs := make([]ai.Message, 0, len(recentDesc))
	for i := len(recentDesc) - 1; i >= 0; i-- {
		m := recentDesc[i]
		providerMsgs = append(providerMsgs, ai.Message{Role: m.Role, Content: m.Content})
	}

	reply, err := provider.Chat(ctx, providerMsgs)
	if err != nil {
		return "", 0, err
	}

	assistantMsg := &Message{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "assistant",
		Content:   reply,
	}
	if err := s.repo.InsertMessage(ctx, assistantMsg); err != nil {
		return "", 0, err
	}
	return reply, assistantMsg.ID, nil
}

func (s *Service) CreateJobOrGetExisting(ctx context.Context, job *Job) (*Job, bool, error) {
	return s.repo.CreateJobOrGetExisting(ctx, job)
}

func (s *Service) InsertUserMessageOrGetExisting(ctx context.Context, userID uint64, sessionID string, content string, key *string) (*Message, bool, error) {
	return s.repo.InsertUserMessageOrGetExisting(ctx, userID, sessionID, content, key)
}
