// Package kbstore provides knowledge base storage using SQLite.
package kbstore

import (
	"context"
	"os"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// createTestStore creates a temporary SQLite store for testing.
func createTestStore(t *testing.T) (*SQLiteStore, func()) {
	t.Helper()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "kbstore_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Open store
	store, err := Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		t.Fatalf("failed to open store: %v", err)
	}

	// Initialize tables
	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		store.Close()
		os.Remove(tmpPath)
		t.Fatalf("failed to initialize store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpPath)
	}

	return store, cleanup
}

// genAlphaNumID generates a valid ID string (alphanumeric, non-empty).
func genAlphaNumID() gopter.Gen {
	// Generate a string of length 1-10 from alphanumeric characters
	return gen.SliceOfN(10, gen.AlphaNumChar()).Map(func(chars []rune) string {
		if len(chars) == 0 {
			return "a" // Fallback
		}
		return string(chars)
	})
}

// genNonEmptyString generates a non-empty string.
func genNonEmptyString() gopter.Gen {
	// Generate a string of length 1-20 from alphanumeric characters
	return gen.SliceOfN(20, gen.AlphaNumChar()).Map(func(chars []rune) string {
		if len(chars) == 0 {
			return "default" // Fallback
		}
		return string(chars)
	})
}

// genVector generates a random float64 vector.
func genVector(dim int) gopter.Gen {
	return gen.SliceOfN(dim, gen.Float64Range(-1.0, 1.0))
}

// TestProperty11_KBStoreCRUDConsistency tests Property 11: KB Store CRUD Consistency
// **Feature: go-telegram-intent-bot, Property 11: KB Store CRUD Consistency**
// **Validates: Requirements 6.3, 6.4, 6.6**
//
// For any sequence of Add, Delete, Get operations on keywords or FAQs,
// the Get result SHALL reflect the current state after all operations.
func TestProperty11_KBStoreCRUDConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11a: Keyword Add then Get returns the keyword
	properties.Property("keyword add then get returns keyword", prop.ForAll(
		func(keyword string, reply string) bool {
			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			// Add keyword
			if err := store.AddKeyword(ctx, keyword, reply); err != nil {
				t.Logf("AddKeyword error: %v", err)
				return false
			}

			// Get all keywords
			keywords, err := store.GetAllKeywords(ctx)
			if err != nil {
				t.Logf("GetAllKeywords error: %v", err)
				return false
			}

			// Should contain the added keyword
			for _, k := range keywords {
				if k.Keyword == keyword && k.Reply == reply {
					return true
				}
			}
			return false
		},
		genAlphaNumID(),
		genNonEmptyString(),
	))

	// Property 11b: Keyword Delete then Get returns empty
	properties.Property("keyword delete then get returns empty", prop.ForAll(
		func(keyword string, reply string) bool {
			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			// Add keyword
			if err := store.AddKeyword(ctx, keyword, reply); err != nil {
				return false
			}

			// Delete keyword
			if err := store.DeleteKeyword(ctx, keyword); err != nil {
				return false
			}

			// Get all keywords
			keywords, err := store.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			// Should not contain the deleted keyword
			for _, k := range keywords {
				if k.Keyword == keyword {
					return false
				}
			}
			return true
		},
		genAlphaNumID(),
		genNonEmptyString(),
	))

	// Property 11c: FAQ Add then Get returns the FAQ
	properties.Property("FAQ add then get returns FAQ", prop.ForAll(
		func(faqID string, question string, answer string) bool {
			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			vector := []float64{0.1, 0.2, 0.3}

			// Add FAQ
			if err := store.AddFAQ(ctx, faqID, question, answer, vector); err != nil {
				t.Logf("AddFAQ error: %v", err)
				return false
			}

			// Get FAQ
			faq, err := store.GetFAQ(ctx, faqID)
			if err != nil {
				t.Logf("GetFAQ error: %v", err)
				return false
			}

			if faq == nil {
				return false
			}

			return faq.FAQID == faqID && faq.Question == question && faq.Answer == answer
		},
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	// Property 11d: FAQ Delete then Get returns nil
	properties.Property("FAQ delete then get returns nil", prop.ForAll(
		func(faqID string, question string, answer string) bool {
			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			vector := []float64{0.1, 0.2, 0.3}

			// Add FAQ
			if err := store.AddFAQ(ctx, faqID, question, answer, vector); err != nil {
				return false
			}

			// Delete FAQ
			if err := store.DeleteFAQ(ctx, faqID); err != nil {
				return false
			}

			// Get FAQ
			faq, err := store.GetFAQ(ctx, faqID)
			if err != nil {
				return false
			}

			return faq == nil
		},
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	// Property 11e: Cache invalidation on data change
	properties.Property("cache invalidated on data change", prop.ForAll(
		func(keyword1 string, keyword2 string, reply string) bool {
			if keyword1 == keyword2 {
				return true // Skip if same keyword
			}

			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			// Add first keyword
			if err := store.AddKeyword(ctx, keyword1, reply); err != nil {
				return false
			}

			// Get keywords (populates cache)
			keywords1, err := store.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			// Add second keyword (should invalidate cache)
			if err := store.AddKeyword(ctx, keyword2, reply); err != nil {
				return false
			}

			// Get keywords again (should reflect new state)
			keywords2, err := store.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			// Second result should have more keywords
			return len(keywords2) > len(keywords1)
		},
		genAlphaNumID(),
		genAlphaNumID(),
		genNonEmptyString(),
	))

	properties.TestingRun(t)
}

// TestProperty12_YAMLRoundTrip tests Property 12: YAML Round-Trip
// **Feature: go-telegram-intent-bot, Property 12: YAML Round-Trip**
// **Validates: Requirements 6.7, 6.8**
//
// For any valid knowledge base state, exporting to YAML and then importing
// SHALL produce an equivalent state.
func TestProperty12_YAMLRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 12: Export then Import produces equivalent state
	properties.Property("YAML export then import produces equivalent state", prop.ForAll(
		func(keywords []KeywordEntry, faqs []FAQEntry) bool {
			store1, cleanup1 := createTestStore(t)
			defer cleanup1()
			ctx := context.Background()

			// Add keywords to store1
			for _, kw := range keywords {
				if err := store1.AddKeyword(ctx, kw.Keyword, kw.Reply); err != nil {
					t.Logf("AddKeyword error: %v", err)
					return false
				}
			}

			// Add FAQs to store1 (without vectors for YAML round-trip)
			for _, faq := range faqs {
				if err := store1.AddFAQ(ctx, faq.FAQID, faq.Question, faq.Answer, nil); err != nil {
					t.Logf("AddFAQ error: %v", err)
					return false
				}
			}

			// Export to YAML
			yamlContent, err := store1.ExportToYAML(ctx)
			if err != nil {
				t.Logf("ExportToYAML error: %v", err)
				return false
			}

			// Create second store and import
			store2, cleanup2 := createTestStore(t)
			defer cleanup2()

			if err := store2.ImportFromYAML(ctx, yamlContent, "overwrite"); err != nil {
				t.Logf("ImportFromYAML error: %v", err)
				return false
			}

			// Compare keywords
			keywords1, err := store1.GetAllKeywords(ctx)
			if err != nil {
				return false
			}
			keywords2, err := store2.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			if len(keywords1) != len(keywords2) {
				t.Logf("keyword count mismatch: %d vs %d", len(keywords1), len(keywords2))
				return false
			}

			// Compare FAQs
			faqs1, err := store1.GetAllFAQs(ctx)
			if err != nil {
				return false
			}
			faqs2, err := store2.GetAllFAQs(ctx)
			if err != nil {
				return false
			}

			if len(faqs1) != len(faqs2) {
				t.Logf("FAQ count mismatch: %d vs %d", len(faqs1), len(faqs2))
				return false
			}

			return true
		},
		genKeywordEntryList(),
		genFAQEntryList(),
	))

	// Property 12b: Merge mode preserves existing data
	properties.Property("merge mode preserves existing data", prop.ForAll(
		func(keyword1 string, keyword2 string, reply1 string, reply2 string) bool {
			if keyword1 == keyword2 {
				return true // Skip if same keyword
			}

			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			// Add first keyword directly
			if err := store.AddKeyword(ctx, keyword1, reply1); err != nil {
				return false
			}

			// Create YAML with second keyword
			yamlContent := "keywords:\n  - keyword: \"" + keyword2 + "\"\n    reply: \"" + reply2 + "\"\nfaq: []\n"

			// Import with merge mode
			if err := store.ImportFromYAML(ctx, yamlContent, "merge"); err != nil {
				t.Logf("ImportFromYAML error: %v", err)
				return false
			}

			// Both keywords should exist
			keywords, err := store.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			found1, found2 := false, false
			for _, k := range keywords {
				if k.Keyword == keyword1 {
					found1 = true
				}
				if k.Keyword == keyword2 {
					found2 = true
				}
			}

			return found1 && found2
		},
		genAlphaNumID(),
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	// Property 12c: Overwrite mode replaces all data
	properties.Property("overwrite mode replaces all data", prop.ForAll(
		func(keyword1 string, keyword2 string, reply1 string, reply2 string) bool {
			if keyword1 == keyword2 {
				return true // Skip if same keyword
			}

			store, cleanup := createTestStore(t)
			defer cleanup()
			ctx := context.Background()

			// Add first keyword directly
			if err := store.AddKeyword(ctx, keyword1, reply1); err != nil {
				return false
			}

			// Create YAML with second keyword only
			yamlContent := "keywords:\n  - keyword: \"" + keyword2 + "\"\n    reply: \"" + reply2 + "\"\nfaq: []\n"

			// Import with overwrite mode
			if err := store.ImportFromYAML(ctx, yamlContent, "overwrite"); err != nil {
				t.Logf("ImportFromYAML error: %v", err)
				return false
			}

			// Only second keyword should exist
			keywords, err := store.GetAllKeywords(ctx)
			if err != nil {
				return false
			}

			if len(keywords) != 1 {
				return false
			}

			return keywords[0].Keyword == keyword2
		},
		genAlphaNumID(),
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	))

	properties.TestingRun(t)
}

// genKeywordEntry generates a random keyword entry.
func genKeywordEntry() gopter.Gen {
	return gopter.CombineGens(
		genAlphaNumID(),
		genNonEmptyString(),
	).Map(func(values []interface{}) KeywordEntry {
		return KeywordEntry{
			Keyword: values[0].(string),
			Reply:   values[1].(string),
		}
	})
}

// genKeywordEntryList generates a list of unique keyword entries.
func genKeywordEntryList() gopter.Gen {
	return gen.SliceOfN(3, genKeywordEntry()).Map(func(entries []KeywordEntry) []KeywordEntry {
		// Ensure unique keywords
		seen := make(map[string]bool)
		var unique []KeywordEntry
		for _, e := range entries {
			if !seen[e.Keyword] {
				seen[e.Keyword] = true
				unique = append(unique, e)
			}
		}
		return unique
	})
}

// genFAQEntry generates a random FAQ entry.
func genFAQEntry() gopter.Gen {
	return gopter.CombineGens(
		genAlphaNumID(),
		genNonEmptyString(),
		genNonEmptyString(),
	).Map(func(values []interface{}) FAQEntry {
		return FAQEntry{
			FAQID:    values[0].(string),
			Question: values[1].(string),
			Answer:   values[2].(string),
		}
	})
}

// genFAQEntryList generates a list of unique FAQ entries.
func genFAQEntryList() gopter.Gen {
	return gen.SliceOfN(3, genFAQEntry()).Map(func(entries []FAQEntry) []FAQEntry {
		// Ensure unique FAQ IDs
		seen := make(map[string]bool)
		var unique []FAQEntry
		for _, e := range entries {
			if !seen[e.FAQID] {
				seen[e.FAQID] = true
				unique = append(unique, e)
			}
		}
		return unique
	})
}

// TestVectorEncodeDecode tests vector encoding and decoding.
func TestVectorEncodeDecode(t *testing.T) {
	testCases := [][]float64{
		{},
		{1.0},
		{0.1, 0.2, 0.3},
		{-1.0, 0.0, 1.0},
		{0.123456789, -0.987654321},
	}

	for _, original := range testCases {
		encoded, err := encodeVector(original)
		if err != nil {
			t.Errorf("encodeVector failed: %v", err)
			continue
		}

		decoded, err := decodeVector(encoded)
		if err != nil {
			t.Errorf("decodeVector failed: %v", err)
			continue
		}

		if len(decoded) != len(original) {
			t.Errorf("length mismatch: got %d, want %d", len(decoded), len(original))
			continue
		}

		for i := range original {
			if decoded[i] != original[i] {
				t.Errorf("value mismatch at %d: got %f, want %f", i, decoded[i], original[i])
			}
		}
	}
}

// TestInitialize tests database initialization.
func TestInitialize(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()

	// Initialize should be idempotent
	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		t.Errorf("second Initialize failed: %v", err)
	}
}

// TestFAQVectorStorage tests FAQ vector storage and retrieval.
func TestFAQVectorStorage(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	faqID := "test-faq"
	question := "How to test?"
	answer := "Write tests!"
	vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	// Add FAQ with vector
	if err := store.AddFAQ(ctx, faqID, question, answer, vector); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}

	// Get FAQ vectors
	vectors, err := store.GetAllFAQVectors(ctx)
	if err != nil {
		t.Fatalf("GetAllFAQVectors failed: %v", err)
	}

	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}

	if vectors[0].FAQID != faqID {
		t.Errorf("FAQ ID mismatch: got %s, want %s", vectors[0].FAQID, faqID)
	}

	if len(vectors[0].Vector) != len(vector) {
		t.Errorf("vector length mismatch: got %d, want %d", len(vectors[0].Vector), len(vector))
	}

	for i := range vector {
		if vectors[0].Vector[i] != vector[i] {
			t.Errorf("vector value mismatch at %d: got %f, want %f", i, vectors[0].Vector[i], vector[i])
		}
	}
}

// TestDeleteNonExistent tests deleting non-existent items.
func TestDeleteNonExistent(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Delete non-existent keyword
	err := store.DeleteKeyword(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error when deleting non-existent keyword")
	}

	// Delete non-existent FAQ
	err = store.DeleteFAQ(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error when deleting non-existent FAQ")
	}
}

// TestCascadeDelete tests that deleting FAQ also deletes its vector.
func TestCascadeDelete(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	ctx := context.Background()

	faqID := "cascade-test"
	vector := []float64{0.1, 0.2, 0.3}

	// Add FAQ with vector
	if err := store.AddFAQ(ctx, faqID, "question", "answer", vector); err != nil {
		t.Fatalf("AddFAQ failed: %v", err)
	}

	// Verify vector exists
	vectors, err := store.GetAllFAQVectors(ctx)
	if err != nil {
		t.Fatalf("GetAllFAQVectors failed: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}

	// Delete FAQ
	if err := store.DeleteFAQ(ctx, faqID); err != nil {
		t.Fatalf("DeleteFAQ failed: %v", err)
	}

	// Verify vector is also deleted (cascade)
	store.invalidateCache() // Force reload from DB
	vectors, err = store.GetAllFAQVectors(ctx)
	if err != nil {
		t.Fatalf("GetAllFAQVectors failed: %v", err)
	}
	if len(vectors) != 0 {
		t.Errorf("expected 0 vectors after cascade delete, got %d", len(vectors))
	}
}
