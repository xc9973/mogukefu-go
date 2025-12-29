// Package keyword provides keyword matching functionality.
package keyword

import (
	"strings"
	"sync"
)

// Entry represents a keyword and its associated reply.
type Entry struct {
	Keyword string
	Reply   string
}

// Matcher defines the interface for keyword matching.
type Matcher interface {
	// Match checks if text contains a keyword and returns the matched entry.
	// If multiple keywords match, returns the first one in configuration order.
	// Returns nil if no keyword matches.
	Match(text string) *Entry

	// UpdateKeywords updates the keyword list (hot reload).
	// Subsequent Match calls will use the new keyword list.
	UpdateKeywords(keywords []Entry)
}

// DefaultMatcher is the default implementation of Matcher.
// It is thread-safe and supports hot updates.
type DefaultMatcher struct {
	mu       sync.RWMutex
	keywords []Entry
}

// NewMatcher creates a new DefaultMatcher with the given keywords.
func NewMatcher(keywords []Entry) *DefaultMatcher {
	m := &DefaultMatcher{}
	m.UpdateKeywords(keywords)
	return m
}

// Match checks if text contains a keyword and returns the matched entry.
// If multiple keywords match, returns the first one in configuration order.
// Returns nil if no keyword matches.
//
// Validates: Requirements 2.1, 2.2, 2.3
func (m *DefaultMatcher) Match(text string) *Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Convert text to lowercase for case-insensitive matching
	lowerText := strings.ToLower(text)

	// Check keywords in configuration order
	for i := range m.keywords {
		lowerKeyword := strings.ToLower(m.keywords[i].Keyword)
		if strings.Contains(lowerText, lowerKeyword) {
			// Return a copy to avoid data races
			return &Entry{
				Keyword: m.keywords[i].Keyword,
				Reply:   m.keywords[i].Reply,
			}
		}
	}

	return nil
}

// UpdateKeywords updates the keyword list (hot reload).
// Subsequent Match calls will use the new keyword list.
//
// Validates: Requirements 2.4
func (m *DefaultMatcher) UpdateKeywords(keywords []Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a copy of the keywords slice to avoid external modifications
	m.keywords = make([]Entry, len(keywords))
	copy(m.keywords, keywords)
}
