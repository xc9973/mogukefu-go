// Package vector provides vector storage and similarity search functionality.
package vector

import (
	"context"
	"sync"

	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/kbstore"
)

// FAQEntry represents a FAQ item.
type FAQEntry struct {
	FAQID    string
	Question string
	Answer   string
}

// FAQMatch represents a matched FAQ with its similarity score.
type FAQMatch struct {
	FAQ        FAQEntry
	Similarity float64
}

// Store defines the interface for vector storage and search.
type Store interface {
	// Search finds the most similar FAQ above the threshold.
	// Returns nil if no FAQ exceeds the threshold.
	Search(ctx context.Context, queryVector []float64, threshold float64) (*FAQMatch, error)

	// AddFAQ adds a FAQ with its vector.
	AddFAQ(ctx context.Context, faq FAQEntry, vector []float64) error

	// DeleteFAQ removes a FAQ by ID.
	DeleteFAQ(ctx context.Context, faqID string) error

	// GetAllFAQs returns all FAQs.
	GetAllFAQs(ctx context.Context) ([]FAQEntry, error)

	// GetFAQ returns a single FAQ by ID.
	GetFAQ(ctx context.Context, faqID string) (*FAQEntry, error)

	// RefreshCache reloads vectors from the underlying store.
	RefreshCache(ctx context.Context) error
}

// VectorStore implements Store using KBStore for persistence.
type VectorStore struct {
	kbStore kbstore.Store

	// In-memory cache for fast search
	cacheMu     sync.RWMutex
	faqCache    map[string]FAQEntry  // faq_id -> FAQEntry
	vectorCache map[string][]float64 // faq_id -> vector
	cacheLoaded bool
}

// New creates a new VectorStore with the given KBStore.
func New(kbStore kbstore.Store) *VectorStore {
	return &VectorStore{
		kbStore:     kbStore,
		faqCache:    make(map[string]FAQEntry),
		vectorCache: make(map[string][]float64),
	}
}


// Search finds the most similar FAQ above the threshold.
// Implements Requirements 3.7, 3.8
func (s *VectorStore) Search(ctx context.Context, queryVector []float64, threshold float64) (*FAQMatch, error) {
	if err := s.ensureCacheLoaded(ctx); err != nil {
		return nil, err
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	var bestMatch *FAQMatch
	var bestSimilarity float64 = -1

	for faqID, vector := range s.vectorCache {
		similarity := embedding.CosineSimilarity(queryVector, vector)

		if similarity > bestSimilarity {
			bestSimilarity = similarity
			faq := s.faqCache[faqID]
			bestMatch = &FAQMatch{
				FAQ: FAQEntry{
					FAQID:    faq.FAQID,
					Question: faq.Question,
					Answer:   faq.Answer,
				},
				Similarity: similarity,
			}
		}
	}

	// Return nil if best match doesn't exceed threshold
	if bestMatch == nil || bestMatch.Similarity <= threshold {
		return nil, nil
	}

	return bestMatch, nil
}

// AddFAQ adds a FAQ with its vector.
// Implements Requirements 3.2, 3.3
func (s *VectorStore) AddFAQ(ctx context.Context, faq FAQEntry, vector []float64) error {
	// Persist to database
	if err := s.kbStore.AddFAQ(ctx, faq.FAQID, faq.Question, faq.Answer, vector); err != nil {
		return err
	}

	// Update cache
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.faqCache[faq.FAQID] = faq
	if len(vector) > 0 {
		s.vectorCache[faq.FAQID] = vector
	}

	return nil
}

// DeleteFAQ removes a FAQ by ID.
// Implements Requirements 3.4
func (s *VectorStore) DeleteFAQ(ctx context.Context, faqID string) error {
	// Delete from database
	if err := s.kbStore.DeleteFAQ(ctx, faqID); err != nil {
		return err
	}

	// Update cache
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	delete(s.faqCache, faqID)
	delete(s.vectorCache, faqID)

	return nil
}

// GetAllFAQs returns all FAQs.
func (s *VectorStore) GetAllFAQs(ctx context.Context) ([]FAQEntry, error) {
	if err := s.ensureCacheLoaded(ctx); err != nil {
		return nil, err
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	faqs := make([]FAQEntry, 0, len(s.faqCache))
	for _, faq := range s.faqCache {
		faqs = append(faqs, faq)
	}

	return faqs, nil
}

// GetFAQ returns a single FAQ by ID.
func (s *VectorStore) GetFAQ(ctx context.Context, faqID string) (*FAQEntry, error) {
	if err := s.ensureCacheLoaded(ctx); err != nil {
		return nil, err
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	faq, exists := s.faqCache[faqID]
	if !exists {
		return nil, nil
	}

	return &FAQEntry{
		FAQID:    faq.FAQID,
		Question: faq.Question,
		Answer:   faq.Answer,
	}, nil
}

// RefreshCache reloads all data from the database.
func (s *VectorStore) RefreshCache(ctx context.Context) error {
	// Load FAQs
	faqs, err := s.kbStore.GetAllFAQs(ctx)
	if err != nil {
		return err
	}

	// Load vectors
	vectors, err := s.kbStore.GetAllFAQVectors(ctx)
	if err != nil {
		return err
	}

	// Build new cache
	newFAQCache := make(map[string]FAQEntry, len(faqs))
	for _, faq := range faqs {
		newFAQCache[faq.FAQID] = FAQEntry{
			FAQID:    faq.FAQID,
			Question: faq.Question,
			Answer:   faq.Answer,
		}
	}

	newVectorCache := make(map[string][]float64, len(vectors))
	for _, v := range vectors {
		newVectorCache[v.FAQID] = v.Vector
	}

	// Swap cache atomically
	s.cacheMu.Lock()
	s.faqCache = newFAQCache
	s.vectorCache = newVectorCache
	s.cacheLoaded = true
	s.cacheMu.Unlock()

	return nil
}

// ensureCacheLoaded loads the cache if not already loaded.
func (s *VectorStore) ensureCacheLoaded(ctx context.Context) error {
	s.cacheMu.RLock()
	loaded := s.cacheLoaded
	s.cacheMu.RUnlock()

	if loaded {
		return nil
	}

	return s.RefreshCache(ctx)
}
