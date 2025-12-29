// Package bot provides Telegram bot integration.
package bot

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/xc9973/mogukefu-go/internal/handler"
	"github.com/xc9973/mogukefu-go/internal/keyword"
	"github.com/xc9973/mogukefu-go/internal/vector"
)

// mockEmbeddingClient is a mock implementation of embedding.Client for testing.
type mockEmbeddingClient struct {
	embedFunc func(ctx context.Context, text string) ([]float64, error)
}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for i := range texts {
		result[i] = []float64{0.1, 0.2, 0.3}
	}
	return result, nil
}

// mockVectorStore is a mock implementation of vector.Store for testing.
type mockVectorStore struct {
	faqs map[string]vector.FAQEntry
}

func newMockVectorStore() *mockVectorStore {
	return &mockVectorStore{
		faqs: make(map[string]vector.FAQEntry),
	}
}

func (m *mockVectorStore) Search(ctx context.Context, queryVector []float64, threshold float64) (*vector.FAQMatch, error) {
	return nil, nil
}

func (m *mockVectorStore) AddFAQ(ctx context.Context, faq vector.FAQEntry, vec []float64) error {
	m.faqs[faq.FAQID] = faq
	return nil
}

func (m *mockVectorStore) DeleteFAQ(ctx context.Context, faqID string) error {
	delete(m.faqs, faqID)
	return nil
}

func (m *mockVectorStore) GetAllFAQs(ctx context.Context) ([]vector.FAQEntry, error) {
	result := make([]vector.FAQEntry, 0, len(m.faqs))
	for _, faq := range m.faqs {
		result = append(result, faq)
	}
	return result, nil
}

func (m *mockVectorStore) GetFAQ(ctx context.Context, faqID string) (*vector.FAQEntry, error) {
	faq, ok := m.faqs[faqID]
	if !ok {
		return nil, nil
	}
	return &faq, nil
}

func (m *mockVectorStore) RefreshCache(ctx context.Context) error {
	return nil
}

// mockKBStore is a mock implementation of kbstore.Store for testing.
type mockKBStore struct {
	keywords map[string]string
}

func newMockKBStore() *mockKBStore {
	return &mockKBStore{
		keywords: make(map[string]string),
	}
}

func (m *mockKBStore) Initialize(ctx context.Context) error {
	return nil
}

func (m *mockKBStore) AddKeyword(ctx context.Context, kw, reply string) error {
	m.keywords[kw] = reply
	return nil
}

func (m *mockKBStore) DeleteKeyword(ctx context.Context, kw string) error {
	delete(m.keywords, kw)
	return nil
}

func (m *mockKBStore) GetAllKeywords(ctx context.Context) ([]struct {
	Keyword string
	Reply   string
}, error) {
	result := make([]struct {
		Keyword string
		Reply   string
	}, 0, len(m.keywords))
	for kw, reply := range m.keywords {
		result = append(result, struct {
			Keyword string
			Reply   string
		}{Keyword: kw, Reply: reply})
	}
	return result, nil
}

func (m *mockKBStore) AddFAQ(ctx context.Context, faqID, question, answer string, vec []float64) error {
	return nil
}

func (m *mockKBStore) DeleteFAQ(ctx context.Context, faqID string) error {
	return nil
}

func (m *mockKBStore) GetFAQ(ctx context.Context, faqID string) (*struct {
	FAQID    string
	Question string
	Answer   string
}, error) {
	return nil, nil
}

func (m *mockKBStore) GetAllFAQs(ctx context.Context) ([]struct {
	FAQID    string
	Question string
	Answer   string
}, error) {
	return nil, nil
}

func (m *mockKBStore) GetAllFAQVectors(ctx context.Context) ([]struct {
	FAQID  string
	Vector []float64
}, error) {
	return nil, nil
}

func (m *mockKBStore) ExportToYAML(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockKBStore) ImportFromYAML(ctx context.Context, yamlContent string, mode string) error {
	return nil
}

func (m *mockKBStore) Close() error {
	return nil
}

// TestAdminCommandsIsAdmin tests the admin permission check.
func TestAdminCommandsIsAdmin(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adminIDs := []int64{123, 456, 789}

	adminCmds := NewAdminCommands(nil, Dependencies{}, adminIDs, logger)

	tests := []struct {
		name     string
		userID   int64
		expected bool
	}{
		{"admin user 123", 123, true},
		{"admin user 456", 456, true},
		{"admin user 789", 789, true},
		{"non-admin user", 999, false},
		{"zero user", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adminCmds.isAdmin(tt.userID)
			if result != tt.expected {
				t.Errorf("isAdmin(%d) = %v, want %v", tt.userID, result, tt.expected)
			}
		})
	}
}

// TestDependenciesSetup tests that dependencies can be properly configured.
func TestDependenciesSetup(t *testing.T) {
	// Create mock dependencies
	keywordMatcher := keyword.NewMatcher([]keyword.Entry{
		{Keyword: "test", Reply: "test reply"},
	})
	embeddingClient := &mockEmbeddingClient{}
	vectorStore := newMockVectorStore()

	messageHandler := handler.New(
		keywordMatcher,
		vectorStore,
		embeddingClient,
		handler.Config{
			ShortMessageThreshold: 10,
			SimilarityThreshold:   0.75,
		},
	)

	deps := Dependencies{
		VectorStore:     vectorStore,
		EmbeddingClient: embeddingClient,
		KeywordMatcher:  keywordMatcher,
		MessageHandler:  messageHandler,
	}

	// Verify dependencies are set
	if deps.VectorStore == nil {
		t.Error("VectorStore should not be nil")
	}
	if deps.EmbeddingClient == nil {
		t.Error("EmbeddingClient should not be nil")
	}
	if deps.KeywordMatcher == nil {
		t.Error("KeywordMatcher should not be nil")
	}
	if deps.MessageHandler == nil {
		t.Error("MessageHandler should not be nil")
	}
}

// TestConfigSetup tests that bot config can be properly configured.
func TestConfigSetup(t *testing.T) {
	cfg := Config{
		Token:                 "test-token",
		AdminIDs:              []int64{123, 456},
		SimilarityThreshold:   0.75,
		ShortMessageThreshold: 10,
	}

	if cfg.Token != "test-token" {
		t.Errorf("Token = %s, want test-token", cfg.Token)
	}
	if len(cfg.AdminIDs) != 2 {
		t.Errorf("AdminIDs length = %d, want 2", len(cfg.AdminIDs))
	}
	if cfg.SimilarityThreshold != 0.75 {
		t.Errorf("SimilarityThreshold = %f, want 0.75", cfg.SimilarityThreshold)
	}
	if cfg.ShortMessageThreshold != 10 {
		t.Errorf("ShortMessageThreshold = %d, want 10", cfg.ShortMessageThreshold)
	}
}
