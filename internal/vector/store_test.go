// Package vector provides vector storage and similarity search functionality.
package vector

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/kbstore"
)

// createTestVectorStore creates a temporary VectorStore for testing.
func createTestVectorStore(t *testing.T) (*VectorStore, func()) {
	t.Helper()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "vector_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Open kbstore
	kbStore, err := kbstore.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		t.Fatalf("failed to open kbstore: %v", err)
	}

	// Initialize tables
	ctx := context.Background()
	if err := kbStore.Initialize(ctx); err != nil {
		kbStore.Close()
		os.Remove(tmpPath)
		t.Fatalf("failed to initialize kbstore: %v", err)
	}

	// Create vector store
	vectorStore := New(kbStore)

	cleanup := func() {
		kbStore.Close()
		os.Remove(tmpPath)
	}

	return vectorStore, cleanup
}

// genAlphaNumID generates a valid ID string (alphanumeric, non-empty).
func genAlphaNumID() gopter.Gen {
	return gen.SliceOfN(10, gen.AlphaNumChar()).Map(func(chars []rune) string {
		if len(chars) == 0 {
			return "a"
		}
		return string(chars)
	})
}

// genNonEmptyString generates a non-empty string.
func genNonEmptyString() gopter.Gen {
	return gen.SliceOfN(20, gen.AlphaNumChar()).Map(func(chars []rune) string {
		if len(chars) == 0 {
			return "default"
		}
		return string(chars)
	})
}


// genNormalizedVector generates a normalized vector of given dimension.
func genNormalizedVector(dim int) gopter.Gen {
	return gen.SliceOfN(dim, gen.Float64Range(-1.0, 1.0)).Map(func(v []float64) []float64 {
		// Normalize the vector
		var norm float64
		for _, x := range v {
			norm += x * x
		}
		if norm == 0 {
			// Return a unit vector if all zeros
			result := make([]float64, len(v))
			if len(result) > 0 {
				result[0] = 1.0
			}
			return result
		}
		norm = math.Sqrt(norm)
		result := make([]float64, len(v))
		for i, x := range v {
			result[i] = x / norm
		}
		return result
	})
}

// TestProperty4_FAQStorageConsistency tests Property 4: FAQ Storage Consistency
// **Feature: go-telegram-intent-bot, Property 4: FAQ Storage Consistency**
// **Validates: Requirements 3.2, 3.3, 3.4**
//
// For any FAQ entry:
// - After AddFAQ, GetFAQ with the same faq_id SHALL return the entry
// - After DeleteFAQ, GetFAQ with the same faq_id SHALL return nil
// - After UpdateFAQ (AddFAQ with same ID), GetFAQ SHALL return the updated entry
func TestProperty4_FAQStorageConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 4a: Add then Get returns the FAQ
	properties.Property("FAQ add then get returns FAQ", prop.ForAll(
		func(faqID string, question string, answer string) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			vector := []float64{0.1, 0.2, 0.3}
			faq := FAQEntry{
				FAQID:    faqID,
				Question: question,
				Answer:   answer,
			}

			// Add FAQ
			if err := store.AddFAQ(ctx, faq, vector); err != nil {
				t.Logf("AddFAQ error: %v", err)
				return false
			}

			// Get FAQ
			result, err := store.GetFAQ(ctx, faqID)
			if err != nil {
				t.Logf("GetFAQ error: %v", err)
				return false
			}

			if result == nil {
				t.Logf("GetFAQ returned nil")
				return false
			}

			return result.FAQID == faqID && result.Question == question && result.Answer == answer
		},
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	// Property 4b: Delete then Get returns nil
	properties.Property("FAQ delete then get returns nil", prop.ForAll(
		func(faqID string, question string, answer string) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			vector := []float64{0.1, 0.2, 0.3}
			faq := FAQEntry{
				FAQID:    faqID,
				Question: question,
				Answer:   answer,
			}

			// Add FAQ
			if err := store.AddFAQ(ctx, faq, vector); err != nil {
				return false
			}

			// Delete FAQ
			if err := store.DeleteFAQ(ctx, faqID); err != nil {
				return false
			}

			// Get FAQ
			result, err := store.GetFAQ(ctx, faqID)
			if err != nil {
				return false
			}

			return result == nil
		},
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	// Property 4c: Update (Add with same ID) then Get returns updated entry
	properties.Property("FAQ update then get returns updated entry", prop.ForAll(
		func(faqID string, question1 string, answer1 string, question2 string, answer2 string) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			vector := []float64{0.1, 0.2, 0.3}

			// Add initial FAQ
			faq1 := FAQEntry{FAQID: faqID, Question: question1, Answer: answer1}
			if err := store.AddFAQ(ctx, faq1, vector); err != nil {
				return false
			}

			// Update FAQ (add with same ID)
			faq2 := FAQEntry{FAQID: faqID, Question: question2, Answer: answer2}
			if err := store.AddFAQ(ctx, faq2, vector); err != nil {
				return false
			}

			// Get FAQ
			result, err := store.GetFAQ(ctx, faqID)
			if err != nil {
				return false
			}

			if result == nil {
				return false
			}

			// Should return updated values
			return result.FAQID == faqID && result.Question == question2 && result.Answer == answer2
		},
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	properties.TestingRun(t)
}


// TestProperty6_VectorSearchThreshold tests Property 6: Vector Search Threshold
// **Feature: go-telegram-intent-bot, Property 6: Vector Search Threshold**
// **Validates: Requirements 3.7, 3.8**
//
// For any query vector and FAQ vectors:
// - Search SHALL return the FAQ with highest similarity if it exceeds the threshold
// - Search SHALL return nil if no FAQ exceeds the threshold
func TestProperty6_VectorSearchThreshold(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	const vectorDim = 8

	// Property 6a: Search returns highest similarity FAQ above threshold
	properties.Property("search returns highest similarity FAQ above threshold", prop.ForAll(
		func(faqID1 string, faqID2 string) bool {
			if faqID1 == faqID2 {
				return true // Skip if same ID
			}

			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			// Create two FAQs with different vectors
			// FAQ1: vector pointing in direction [1, 0, 0, ...]
			vector1 := make([]float64, vectorDim)
			vector1[0] = 1.0

			// FAQ2: vector pointing in direction [0, 1, 0, ...]
			vector2 := make([]float64, vectorDim)
			vector2[1] = 1.0

			faq1 := FAQEntry{FAQID: faqID1, Question: "Q1", Answer: "A1"}
			faq2 := FAQEntry{FAQID: faqID2, Question: "Q2", Answer: "A2"}

			if err := store.AddFAQ(ctx, faq1, vector1); err != nil {
				return false
			}
			if err := store.AddFAQ(ctx, faq2, vector2); err != nil {
				return false
			}

			// Query with vector similar to FAQ1
			queryVector := make([]float64, vectorDim)
			queryVector[0] = 0.9
			queryVector[1] = 0.1
			// Normalize
			norm := math.Sqrt(0.9*0.9 + 0.1*0.1)
			queryVector[0] /= norm
			queryVector[1] /= norm

			// Search with low threshold
			result, err := store.Search(ctx, queryVector, 0.5)
			if err != nil {
				t.Logf("Search error: %v", err)
				return false
			}

			if result == nil {
				t.Logf("Search returned nil")
				return false
			}

			// Should return FAQ1 (higher similarity)
			return result.FAQ.FAQID == faqID1
		},
		genAlphaNumID(),
		genAlphaNumID(),
	))

	// Property 6b: Search returns nil when no FAQ exceeds threshold
	properties.Property("search returns nil when no FAQ exceeds threshold", prop.ForAll(
		func(faqID string) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			// Create FAQ with vector pointing in one direction
			vector := make([]float64, vectorDim)
			vector[0] = 1.0

			faq := FAQEntry{FAQID: faqID, Question: "Q", Answer: "A"}
			if err := store.AddFAQ(ctx, faq, vector); err != nil {
				return false
			}

			// Query with orthogonal vector (similarity = 0)
			queryVector := make([]float64, vectorDim)
			queryVector[1] = 1.0

			// Search with high threshold
			result, err := store.Search(ctx, queryVector, 0.9)
			if err != nil {
				t.Logf("Search error: %v", err)
				return false
			}

			// Should return nil (similarity 0 < threshold 0.9)
			return result == nil
		},
		genAlphaNumID(),
	))

	// Property 6c: Search returns nil for empty store
	properties.Property("search returns nil for empty store", prop.ForAll(
		func(dummy int) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			queryVector := make([]float64, vectorDim)
			queryVector[0] = 1.0

			result, err := store.Search(ctx, queryVector, 0.5)
			if err != nil {
				t.Logf("Search error: %v", err)
				return false
			}

			return result == nil
		},
		gen.Int(),
	))

	// Property 6d: Similarity score is correctly calculated
	properties.Property("similarity score is correctly calculated", prop.ForAll(
		func(faqID string) bool {
			store, cleanup := createTestVectorStore(t)
			defer cleanup()
			ctx := context.Background()

			// Create FAQ with known vector
			vector := make([]float64, vectorDim)
			vector[0] = 1.0

			faq := FAQEntry{FAQID: faqID, Question: "Q", Answer: "A"}
			if err := store.AddFAQ(ctx, faq, vector); err != nil {
				return false
			}

			// Query with same vector (similarity should be 1.0)
			queryVector := make([]float64, vectorDim)
			queryVector[0] = 1.0

			result, err := store.Search(ctx, queryVector, 0.0)
			if err != nil {
				return false
			}

			if result == nil {
				return false
			}

			// Similarity should be very close to 1.0
			expectedSimilarity := embedding.CosineSimilarity(queryVector, vector)
			return math.Abs(result.Similarity-expectedSimilarity) < 0.0001
		},
		genAlphaNumID(),
	))

	properties.TestingRun(t)
}

// TestVectorStoreBasic tests basic VectorStore operations.
func TestVectorStoreBasic(t *testing.T) {
	store, cleanup := createTestVectorStore(t)
	defer cleanup()
	ctx := context.Background()

	// Test AddFAQ and GetFAQ
	faq := FAQEntry{
		FAQID:    "test-faq",
		Question: "How to test?",
		Answer:   "Write tests!",
	}
	vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	if err := store.AddFAQ(ctx, faq, vector); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}

	result, err := store.GetFAQ(ctx, "test-faq")
	if err != nil {
		t.Fatalf("GetFAQ failed: %v", err)
	}

	if result == nil {
		t.Fatal("GetFAQ returned nil")
	}

	if result.FAQID != faq.FAQID || result.Question != faq.Question || result.Answer != faq.Answer {
		t.Errorf("FAQ mismatch: got %+v, want %+v", result, faq)
	}

	// Test GetAllFAQs
	faqs, err := store.GetAllFAQs(ctx)
	if err != nil {
		t.Fatalf("GetAllFAQs failed: %v", err)
	}

	if len(faqs) != 1 {
		t.Errorf("expected 1 FAQ, got %d", len(faqs))
	}

	// Test DeleteFAQ
	if err := store.DeleteFAQ(ctx, "test-faq"); err != nil {
		t.Fatalf("DeleteFAQ failed: %v", err)
	}

	result, err = store.GetFAQ(ctx, "test-faq")
	if err != nil {
		t.Fatalf("GetFAQ after delete failed: %v", err)
	}

	if result != nil {
		t.Error("GetFAQ should return nil after delete")
	}
}

// TestVectorSearch tests vector similarity search.
func TestVectorSearch(t *testing.T) {
	store, cleanup := createTestVectorStore(t)
	defer cleanup()
	ctx := context.Background()

	// Add FAQs with different vectors
	faq1 := FAQEntry{FAQID: "faq1", Question: "Q1", Answer: "A1"}
	vector1 := []float64{1.0, 0.0, 0.0}

	faq2 := FAQEntry{FAQID: "faq2", Question: "Q2", Answer: "A2"}
	vector2 := []float64{0.0, 1.0, 0.0}

	if err := store.AddFAQ(ctx, faq1, vector1); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}
	if err := store.AddFAQ(ctx, faq2, vector2); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}

	// Search with vector similar to faq1
	queryVector := []float64{0.9, 0.1, 0.0}
	result, err := store.Search(ctx, queryVector, 0.5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result == nil {
		t.Fatal("Search returned nil")
	}

	if result.FAQ.FAQID != "faq1" {
		t.Errorf("expected faq1, got %s", result.FAQ.FAQID)
	}

	// Search with very high threshold (should return nil)
	// The similarity between [0.9, 0.1, 0.0] and [1.0, 0.0, 0.0] is about 0.994
	// So we need a threshold higher than that
	result, err = store.Search(ctx, queryVector, 0.999)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result != nil {
		t.Error("Search should return nil with very high threshold")
	}
}

// TestRefreshCache tests cache refresh functionality.
func TestRefreshCache(t *testing.T) {
	store, cleanup := createTestVectorStore(t)
	defer cleanup()
	ctx := context.Background()

	// Add FAQ
	faq := FAQEntry{FAQID: "cache-test", Question: "Q", Answer: "A"}
	vector := []float64{1.0, 0.0, 0.0}

	if err := store.AddFAQ(ctx, faq, vector); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}

	// Refresh cache
	if err := store.RefreshCache(ctx); err != nil {
		t.Fatalf("RefreshCache failed: %v", err)
	}

	// Verify FAQ is in cache
	result, err := store.GetFAQ(ctx, "cache-test")
	if err != nil {
		t.Fatalf("GetFAQ failed: %v", err)
	}

	if result == nil {
		t.Error("GetFAQ returned nil after cache refresh")
	}
}
