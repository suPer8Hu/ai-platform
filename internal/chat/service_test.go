package chat

import (
	"context"
	"testing"
	"time"

	gormsqlite "github.com/glebarez/sqlite"
	"github.com/suPer8Hu/ai-platform/internal/ai"
	"gorm.io/gorm"
)

type recordingProvider struct {
	last []ai.Message
}

func (p *recordingProvider) Chat(ctx context.Context, messages []ai.Message) (string, error) {
	_ = ctx
	// copy to avoid mutations
	p.last = append([]ai.Message(nil), messages...)
	return "ok", nil
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gormsqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Session{}, &Message{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestSendMessage_WritesUserAndAssistant(t *testing.T) {
	db := openTestDB(t)

	repo := NewRepo(db)

	prov := &recordingProvider{}
	reg := ai.NewRegistry()
	reg.Register("fake", func(ctx context.Context, model string) (ai.Provider, error) {
		_ = ctx
		_ = model
		return prov, nil
	})

	svc := NewService(repo, reg, 20)

	// create session
	sess := &Session{
		SessionID: "01TESTSESSIONID00000000000000",
		UserID:    1,
		Provider:  "fake",
		Model:     "default",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	reply, assistantID, err := svc.SendMessage(context.Background(), 1, sess.SessionID, "Hello")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if assistantID == 0 {
		t.Fatalf("expected assistant message id to be set")
	}

	var msgs []Message
	if err := db.Where("session_id = ? AND user_id = ?", sess.SessionID, uint64(1)).
		Order("id ASC").
		Find(&msgs).Error; err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Fatalf("unexpected user msg: role=%q content=%q", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "ok" {
		t.Fatalf("unexpected assistant msg: role=%q content=%q", msgs[1].Role, msgs[1].Content)
	}
}

func TestSendMessage_UsesContextWindow(t *testing.T) {
	db := openTestDB(t)

	repo := NewRepo(db)

	prov := &recordingProvider{}
	reg := ai.NewRegistry()
	reg.Register("fake", func(ctx context.Context, model string) (ai.Provider, error) {
		_ = ctx
		_ = model
		return prov, nil
	})

	window := 3
	svc := NewService(repo, reg, window)

	sess := &Session{
		SessionID: "01TESTSESSIONID00000000000001",
		UserID:    2,
		Provider:  "fake",
		Model:     "default",
	}
	if err := repo.CreateSession(context.Background(), sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// seed messages: 5 messages already in history
	for i := 0; i < 5; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if err := repo.InsertMessage(context.Background(), &Message{
			SessionID: sess.SessionID,
			UserID:    2,
			Role:      role,
			Content:   "seed",
		}); err != nil {
			t.Fatalf("seed msg %d: %v", i, err)
		}
	}

	// sending a new message: history grows, but provider should get only `window` most recent msgs
	_, _, err := svc.SendMessage(context.Background(), 2, sess.SessionID, "new")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	if len(prov.last) != window {
		t.Fatalf("expected provider to receive %d messages, got %d", window, len(prov.last))
	}
	// The newest message in provider input should be the user message we just sent.
	if prov.last[len(prov.last)-1].Role != "user" || prov.last[len(prov.last)-1].Content != "new" {
		t.Fatalf("expected last provider msg to be new user msg, got role=%q content=%q",
			prov.last[len(prov.last)-1].Role, prov.last[len(prov.last)-1].Content)
	}
}
