// Package handler provides message handling functionality.
package handler

import (
	"context"
	"strings"

	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/keyword"
	"github.com/xc9973/mogukefu-go/internal/vector"
)

// Result represents the result of message handling.
type Result struct {
	ShouldReply    bool
	ReplyText      string
	MatchedKeyword string
	MatchedFAQID   string
	Similarity     float64
}

// Handler defines the interface for message handling.
type Handler interface {
	// Handle processes a message and returns the result.
	// Returns empty ReplyText if no reply should be sent.
	Handle(ctx context.Context, text string) (*Result, error)
}

// Config holds configuration for the message handler.
type Config struct {
	ShortMessageThreshold int
	SimilarityThreshold   float64
}

// MessageHandler implements the Handler interface.
type MessageHandler struct {
	keywordMatcher  keyword.Matcher
	vectorStore     vector.Store
	embeddingClient embedding.Client
	config          Config
}

// New creates a new MessageHandler.
func New(
	keywordMatcher keyword.Matcher,
	vectorStore vector.Store,
	embeddingClient embedding.Client,
	cfg Config,
) *MessageHandler {
	return &MessageHandler{
		keywordMatcher:  keywordMatcher,
		vectorStore:     vectorStore,
		embeddingClient: embeddingClient,
		config:          cfg,
	}
}


// Handle processes a message and returns the result.
// Implements Requirements 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8
func (h *MessageHandler) Handle(ctx context.Context, text string) (*Result, error) {
	result := &Result{}

	// Requirement 4.1: Ignore messages shorter than 2 characters
	if len(text) < 2 {
		return result, nil
	}

	// Requirement 4.2: Ignore command messages (starting with "/")
	if strings.HasPrefix(text, "/") {
		return result, nil
	}

	// Requirement 4.3, 4.4: Try keyword matching first
	if entry := h.keywordMatcher.Match(text); entry != nil {
		result.ShouldReply = true
		result.ReplyText = entry.Reply
		result.MatchedKeyword = entry.Keyword
		return result, nil
	}

	// Requirement 4.5: Ignore short messages if keyword not matched
	if len(text) <= h.config.ShortMessageThreshold {
		return result, nil
	}

	// Requirement 4.6: Perform vector similarity search for longer messages
	queryVector, err := h.embeddingClient.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Requirement 4.7, 4.8: Search for matching FAQ
	match, err := h.vectorStore.Search(ctx, queryVector, h.config.SimilarityThreshold)
	if err != nil {
		return nil, err
	}

	if match != nil {
		result.ShouldReply = true
		result.ReplyText = match.FAQ.Answer
		result.MatchedFAQID = match.FAQ.FAQID
		result.Similarity = match.Similarity
		return result, nil
	}

	// Requirement 4.8: No match found, return empty (silent)
	return result, nil
}

// runeLen returns the number of runes (characters) in a string.
// This is used for proper Unicode character counting.
func runeLen(s string) int {
	return len([]rune(s))
}
