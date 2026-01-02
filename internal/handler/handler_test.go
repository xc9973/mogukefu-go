// Package handler provides message handling functionality.
package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
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
	// Return a simple deterministic vector based on text length
	return []float64{float64(len(text)), 0.5, 0.5}, nil
}

func (m *mockEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for i, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result[i] = vec
	}
	return result, nil
}

// mockVectorStore is a mock implementation of vector.Store for testing.
type mockVectorStore struct {
	faqs      map[string]vector.FAQEntry
	vectors   map[string][]float64
	threshold float64
}

func newMockVectorStore() *mockVectorStore {
	return &mockVectorStore{
		faqs:    make(map[string]vector.FAQEntry),
		vectors: make(map[string][]float64),
	}
}


func (m *mockVectorStore) Search(ctx context.Context, queryVector []float64, threshold float64) (*vector.FAQMatch, error) {
	m.threshold = threshold
	var bestMatch *vector.FAQMatch
	var bestSim float64 = -1

	for faqID, vec := range m.vectors {
		// Simple similarity: compare first element
		sim := 1.0 - abs(queryVector[0]-vec[0])/100.0
		if sim > bestSim && sim > threshold {
			bestSim = sim
			faq := m.faqs[faqID]
			bestMatch = &vector.FAQMatch{
				FAQ:        faq,
				Similarity: sim,
			}
		}
	}
	return bestMatch, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (m *mockVectorStore) AddFAQ(ctx context.Context, faq vector.FAQEntry, vec []float64) error {
	m.faqs[faq.FAQID] = faq
	m.vectors[faq.FAQID] = vec
	return nil
}

func (m *mockVectorStore) DeleteFAQ(ctx context.Context, faqID string) error {
	delete(m.faqs, faqID)
	delete(m.vectors, faqID)
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


// genShortMessage generates a message shorter than 2 characters.
func genShortMessage() gopter.Gen {
	return gen.OneConstOf("", "a", "1", " ")
}

// genCommandMessage generates a message starting with "/".
func genCommandMessage() gopter.Gen {
	return gen.AlphaString().Map(func(s string) string {
		return "/" + s
	})
}

// genNormalMessage generates a normal message (not command, length >= 2).
func genNormalMessage(minLen int) gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) >= minLen && !strings.HasPrefix(s, "/")
	})
}

// genKeywordEntry generates a random keyword entry.
func genKeywordEntry() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
	).Map(func(values []interface{}) keyword.Entry {
		return keyword.Entry{
			Keyword: values[0].(string),
			Reply:   values[1].(string),
		}
	})
}

// genKeywordList generates a list of keyword entries.
func genKeywordList() gopter.Gen {
	return gen.SliceOfN(3, genKeywordEntry()).SuchThat(func(entries []keyword.Entry) bool {
		seen := make(map[string]bool)
		for _, e := range entries {
			lower := strings.ToLower(e.Keyword)
			if seen[lower] {
				return false
			}
			seen[lower] = true
		}
		return true
	})
}


// TestProperty7_MessageFiltering tests Property 7: Message Filtering
// **Feature: go-telegram-intent-bot, Property 7: Message Filtering**
// **Validates: Requirements 4.1, 4.2, 4.5**
//
// For any message text:
// - If length < 2, Handle SHALL return empty (no reply)
// - If starts with "/", Handle SHALL return empty (no reply)
// - If length <= short_message_threshold AND no keyword match, Handle SHALL return empty
func TestProperty7_MessageFiltering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 7a: Messages shorter than 2 characters are ignored (Requirement 4.1)
	properties.Property("short messages (< 2 chars) are ignored", prop.ForAll(
		func(msg string) bool {
			matcher := keyword.NewMatcher([]keyword.Entry{})
			store := newMockVectorStore()
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.75,
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			return !result.ShouldReply && result.ReplyText == ""
		},
		genShortMessage(),
	))

	// Property 7b: Command messages (starting with "/") are ignored (Requirement 4.2)
	properties.Property("command messages are ignored", prop.ForAll(
		func(msg string) bool {
			matcher := keyword.NewMatcher([]keyword.Entry{})
			store := newMockVectorStore()
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.75,
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			return !result.ShouldReply && result.ReplyText == ""
		},
		genCommandMessage(),
	))

	// Property 7c: Short messages without keyword match are ignored (Requirement 4.5)
	properties.Property("short messages without keyword match are ignored", prop.ForAll(
		func(threshold int) bool {
			if threshold < 2 || threshold > 50 {
				return true // Skip invalid thresholds
			}
			
			// Create a message exactly at threshold length
			msg := strings.Repeat("a", threshold)
			
			// Use keywords that won't match
			matcher := keyword.NewMatcher([]keyword.Entry{
				{Keyword: "zzzzunmatchable", Reply: "reply"},
			})
			store := newMockVectorStore()
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: threshold,
				SimilarityThreshold:   0.75,
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			return !result.ShouldReply && result.ReplyText == ""
		},
		gen.IntRange(2, 50),
	))

	properties.TestingRun(t)
}


// TestProperty8_KeywordReplyPriority tests Property 8: Keyword Reply Priority
// **Feature: go-telegram-intent-bot, Property 8: Keyword Reply Priority**
// **Validates: Requirements 4.4**
//
// For any message that matches a keyword, Handle SHALL return the keyword's reply text,
// regardless of FAQ similarity.
func TestProperty8_KeywordReplyPriority(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 8: Keyword match returns keyword reply, not FAQ
	properties.Property("keyword match returns keyword reply", prop.ForAll(
		func(kw keyword.Entry) bool {
			if len(kw.Keyword) == 0 {
				return true // Skip empty keywords
			}

			// Create message EXACTLY matching the keyword
			msg := kw.Keyword
			if len(msg) < 2 {
				return true // Skip too short messages
			}

			matcher := keyword.NewMatcher([]keyword.Entry{kw})

			// Add a FAQ that would also match
			store := newMockVectorStore()
			store.AddFAQ(context.Background(), vector.FAQEntry{
				FAQID:    "faq1",
				Question: "test question",
				Answer:   "FAQ answer that should NOT be returned",
			}, []float64{float64(len(msg)), 0.5, 0.5})

			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 1, // Allow short messages
				SimilarityThreshold:   0.0, // Low threshold to ensure FAQ would match
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}

			// Should return keyword reply, not FAQ answer
			return result.ShouldReply &&
				result.ReplyText == kw.Reply &&
				result.MatchedKeyword == kw.Keyword
		},
		genKeywordEntry(),
	))

	properties.TestingRun(t)
}


// TestProperty9_FAQReply tests Property 9: FAQ Reply
// **Feature: go-telegram-intent-bot, Property 9: FAQ Reply**
// **Validates: Requirements 4.7**
//
// For any message that matches an FAQ (similarity > threshold) and does not match any keyword,
// Handle SHALL return the FAQ's answer.
func TestProperty9_FAQReply(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 9: FAQ match returns FAQ answer when no keyword match
	properties.Property("FAQ match returns FAQ answer", prop.ForAll(
		func(faqAnswer string, msgLen int) bool {
			if msgLen < 12 || msgLen > 100 {
				return true // Skip invalid lengths
			}
			
			// Create a message that won't match any keyword
			msg := strings.Repeat("x", msgLen)
			
			// No keywords
			matcher := keyword.NewMatcher([]keyword.Entry{})
			
			// Add a FAQ with matching vector
			store := newMockVectorStore()
			store.AddFAQ(context.Background(), vector.FAQEntry{
				FAQID:    "faq1",
				Question: "test question",
				Answer:   faqAnswer,
			}, []float64{float64(msgLen), 0.5, 0.5}) // Vector matches message length
			
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.5, // Low threshold to ensure match
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			
			// Should return FAQ answer
			return result.ShouldReply && 
				result.ReplyText == faqAnswer && 
				result.MatchedFAQID == "faq1"
		},
		gen.AlphaString(),
		gen.IntRange(12, 100),
	))

	properties.TestingRun(t)
}


// TestProperty10_SilentOnNoMatch tests Property 10: Silent on No Match
// **Feature: go-telegram-intent-bot, Property 10: Silent on No Match**
// **Validates: Requirements 4.8**
//
// For any message that does not match any keyword AND does not match any FAQ
// (similarity <= threshold), Handle SHALL return empty (no reply).
func TestProperty10_SilentOnNoMatch(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 10: No match returns empty (silent)
	properties.Property("no match returns empty", prop.ForAll(
		func(msgLen int) bool {
			if msgLen < 12 || msgLen > 100 {
				return true // Skip invalid lengths
			}
			
			// Create a message that won't match any keyword
			msg := strings.Repeat("y", msgLen)
			
			// No keywords
			matcher := keyword.NewMatcher([]keyword.Entry{})
			
			// Add a FAQ with very different vector (won't match)
			store := newMockVectorStore()
			store.AddFAQ(context.Background(), vector.FAQEntry{
				FAQID:    "faq1",
				Question: "test question",
				Answer:   "FAQ answer",
			}, []float64{float64(msgLen + 1000), 0.5, 0.5}) // Very different vector
			
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.99, // Very high threshold to ensure no match
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			
			// Should return empty (no reply)
			return !result.ShouldReply && result.ReplyText == ""
		},
		gen.IntRange(12, 100),
	))

	// Property 10b: Empty vector store returns empty
	properties.Property("empty vector store returns empty", prop.ForAll(
		func(msgLen int) bool {
			if msgLen < 12 || msgLen > 100 {
				return true // Skip invalid lengths
			}
			
			msg := strings.Repeat("z", msgLen)
			
			matcher := keyword.NewMatcher([]keyword.Entry{})
			store := newMockVectorStore() // Empty store
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.75,
			})

			result, err := handler.Handle(context.Background(), msg)
			if err != nil {
				return false
			}
			
			return !result.ShouldReply && result.ReplyText == ""
		},
		gen.IntRange(12, 100),
	))

	properties.TestingRun(t)
}


// TestHandleEmbeddingError tests error handling when embedding fails.
func TestHandleEmbeddingError(t *testing.T) {
	matcher := keyword.NewMatcher([]keyword.Entry{})
	store := newMockVectorStore()
	client := &mockEmbeddingClient{
		embedFunc: func(ctx context.Context, text string) ([]float64, error) {
			return nil, errors.New("embedding error")
		},
	}
	handler := New(matcher, store, client, Config{
		ShortMessageThreshold: 10,
		SimilarityThreshold:   0.75,
	})

	// Long message that would trigger embedding
	msg := strings.Repeat("a", 20)
	_, err := handler.Handle(context.Background(), msg)
	if err == nil {
		t.Error("expected error when embedding fails")
	}
}

// TestHandleBasicCases tests basic message handling cases.
func TestHandleBasicCases(t *testing.T) {
	testCases := []struct {
		name        string
		msg         string
		keywords    []keyword.Entry
		expectReply bool
		expectText  string
	}{
		{
			name:        "empty message",
			msg:         "",
			keywords:    []keyword.Entry{},
			expectReply: false,
		},
		{
			name:        "single char message",
			msg:         "a",
			keywords:    []keyword.Entry{},
			expectReply: false,
		},
		{
			name:        "single CJK char message",
			msg:         "嗨",
			keywords:    []keyword.Entry{},
			expectReply: false,
		},
		{
			name:        "command message",
			msg:         "/start",
			keywords:    []keyword.Entry{},
			expectReply: false,
		},
		{
			name:        "keyword match",
			msg:         "hello",
			keywords:    []keyword.Entry{{Keyword: "hello", Reply: "hi there"}},
			expectReply: true,
			expectText:  "hi there",
		},
		{
			name:        "keyword partial match (should not match)",
			msg:         "hello world",
			keywords:    []keyword.Entry{{Keyword: "hello", Reply: "hi there"}},
			expectReply: false,
		},
		{
			name:        "short message no keyword",
			msg:         "hi",
			keywords:    []keyword.Entry{{Keyword: "hello", Reply: "hi there"}},
			expectReply: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := keyword.NewMatcher(tc.keywords)
			store := newMockVectorStore()
			client := &mockEmbeddingClient{}
			handler := New(matcher, store, client, Config{
				ShortMessageThreshold: 10,
				SimilarityThreshold:   0.75,
			})

			result, err := handler.Handle(context.Background(), tc.msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ShouldReply != tc.expectReply {
				t.Errorf("expected ShouldReply=%v, got %v", tc.expectReply, result.ShouldReply)
			}

			if tc.expectReply && result.ReplyText != tc.expectText {
				t.Errorf("expected ReplyText=%q, got %q", tc.expectText, result.ReplyText)
			}
		})
	}
}
