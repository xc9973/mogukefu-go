// Package main provides tests for the Vectorize CLI tool.
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xc9973/mogukefu-go/internal/kbstore"
)

// TestComputeFAQHash tests the hash computation for change detection.
func TestComputeFAQHash(t *testing.T) {
	// Same content should produce same hash
	hash1 := computeFAQHash("question1", "answer1")
	hash2 := computeFAQHash("question1", "answer1")
	if hash1 != hash2 {
		t.Errorf("Same content should produce same hash: %s != %s", hash1, hash2)
	}

	// Different content should produce different hash
	hash3 := computeFAQHash("question2", "answer1")
	if hash1 == hash3 {
		t.Errorf("Different content should produce different hash")
	}

	hash4 := computeFAQHash("question1", "answer2")
	if hash1 == hash4 {
		t.Errorf("Different content should produce different hash")
	}
}

// TestTruncateString tests string truncation.
func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short     "},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
	}

	for _, tc := range tests {
		result := truncateString(tc.input, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateString(%q, %d) = %q, expected %q", tc.input, tc.maxLen, result, tc.expected)
		}
	}
}

// TestVerifyDatabase tests database verification.
func TestVerifyDatabase(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "vectorize_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Test with non-existent database
	err = verifyDatabase(dbPath)
	if err == nil {
		t.Error("Expected error for non-existent database")
	}

	// Create a valid database
	ctx := context.Background()
	store, err := kbstore.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// Add a keyword
	if err := store.AddKeyword(ctx, "test", "test reply"); err != nil {
		t.Fatalf("Failed to add keyword: %v", err)
	}

	// Add a FAQ with vector
	vector := make([]float64, 128)
	for i := range vector {
		vector[i] = float64(i) / 128.0
	}
	if err := store.AddFAQ(ctx, "faq1", "question1", "answer1", vector); err != nil {
		t.Fatalf("Failed to add FAQ: %v", err)
	}

	store.Close()

	// Verify should pass
	err = verifyDatabase(dbPath)
	if err != nil {
		t.Errorf("Verification should pass for valid database: %v", err)
	}
}

// TestReadInputYAML tests YAML input parsing.
func TestReadInputYAML(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "vectorize_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test YAML file
	yamlContent := `keywords:
  - keyword: "test"
    reply: "test reply"
faq:
  - faq_id: "faq1"
    question: "What is this?"
    answer: "This is a test."
`
	yamlPath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write YAML file: %v", err)
	}

	// Read and parse
	kb, err := readInputYAML(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}

	if len(kb.Keywords) != 1 {
		t.Errorf("Expected 1 keyword, got %d", len(kb.Keywords))
	}
	if kb.Keywords[0].Keyword != "test" {
		t.Errorf("Expected keyword 'test', got %q", kb.Keywords[0].Keyword)
	}

	if len(kb.FAQ) != 1 {
		t.Errorf("Expected 1 FAQ, got %d", len(kb.FAQ))
	}
	if kb.FAQ[0].FAQID != "faq1" {
		t.Errorf("Expected FAQ ID 'faq1', got %q", kb.FAQ[0].FAQID)
	}
}

// TestProcessKeywords tests keyword processing with incremental updates.
func TestProcessKeywords(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "vectorize_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	store, err := kbstore.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	if err := store.Initialize(ctx); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	// First run - add new keywords
	keywords := []kbstore.KeywordEntry{
		{Keyword: "kw1", Reply: "reply1"},
		{Keyword: "kw2", Reply: "reply2"},
	}
	stats := &ProcessStats{}
	if err := processKeywords(ctx, keywords, store, stats); err != nil {
		t.Fatalf("Failed to process keywords: %v", err)
	}

	if stats.NewKeywords != 2 {
		t.Errorf("Expected 2 new keywords, got %d", stats.NewKeywords)
	}

	// Second run - same keywords (should skip)
	stats2 := &ProcessStats{}
	if err := processKeywords(ctx, keywords, store, stats2); err != nil {
		t.Fatalf("Failed to process keywords: %v", err)
	}

	if stats2.NewKeywords != 0 {
		t.Errorf("Expected 0 new keywords on second run, got %d", stats2.NewKeywords)
	}

	// Third run - update one keyword
	keywords[0].Reply = "updated reply1"
	stats3 := &ProcessStats{}
	if err := processKeywords(ctx, keywords, store, stats3); err != nil {
		t.Fatalf("Failed to process keywords: %v", err)
	}

	if stats3.UpdatedKWs != 1 {
		t.Errorf("Expected 1 updated keyword, got %d", stats3.UpdatedKWs)
	}
}
