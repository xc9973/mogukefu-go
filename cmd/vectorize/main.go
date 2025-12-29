// Package main provides the entry point for the Vectorize CLI tool.
// This tool reads FAQ data from YAML files, generates embeddings using the
// configured API, and stores the results in a SQLite database.
// Implements Requirements 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 8.8, 8.9, 8.10, 8.11
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xc9973/mogukefu-go/internal/config"
	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/kbstore"

	"gopkg.in/yaml.v3"
)

// KnowledgeBaseInput represents the YAML input format for FAQ and keywords.
type KnowledgeBaseInput struct {
	Keywords []kbstore.KeywordEntry `yaml:"keywords"`
	FAQ      []FAQInput             `yaml:"faq"`
}

// FAQInput represents a FAQ entry in the input YAML.
type FAQInput struct {
	FAQID    string `yaml:"faq_id"`
	Question string `yaml:"question"`
	Answer   string `yaml:"answer"`
}

// ProcessStats holds statistics about the vectorization process.
type ProcessStats struct {
	TotalFAQs     int
	NewFAQs       int
	UpdatedFAQs   int
	SkippedFAQs   int
	TotalKeywords int
	NewKeywords   int
	UpdatedKWs    int
	Errors        int
}

func main() {
	// Parse command line arguments
	// Implements Requirements 8.7, 8.8, 8.9, 8.10
	inputPath := flag.String("input", "", "Path to input YAML file (required)")
	outputPath := flag.String("output", "knowledge.db", "Path to output SQLite database")
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	verify := flag.Bool("verify", false, "Verify database integrity instead of importing")
	flag.Parse()

	if *inputPath == "" && !*verify {
		fmt.Fprintln(os.Stderr, "Error: --input is required (unless using --verify)")
		flag.Usage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Handle verify mode
	// Implements Requirement 8.10
	if *verify {
		if err := verifyDatabase(*outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Database verification passed")
		return
	}

	// Run vectorization
	if err := runVectorize(*inputPath, *outputPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runVectorize performs the main vectorization process.
// Implements Requirements 8.2, 8.3, 8.4, 8.5, 8.6
func runVectorize(inputPath, outputPath string, cfg *config.Config) error {
	ctx := context.Background()

	// Read input YAML
	// Implements Requirement 8.2
	fmt.Printf("Reading input file: %s\n", inputPath)
	kb, err := readInputYAML(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	fmt.Printf("  Found %d FAQs and %d keywords\n", len(kb.FAQ), len(kb.Keywords))

	// Open database
	// Implements Requirement 8.4
	fmt.Printf("Opening database: %s\n", outputPath)
	store, err := kbstore.Open(outputPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	// Initialize database schema
	if err := store.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create embedding client
	// Implements Requirement 8.3
	embClient := embedding.NewOpenAIClient(embedding.Config{
		BaseURL: cfg.Embedding.BaseURL,
		APIKey:  cfg.Embedding.APIKey,
		Model:   cfg.Embedding.Model,
		Timeout: 60 * time.Second,
	})

	// Process with incremental updates
	// Implements Requirements 8.5, 8.6
	stats, err := processKnowledgeBase(ctx, kb, store, embClient)
	if err != nil {
		return err
	}

	// Print summary
	printStats(stats)

	return nil
}

// readInputYAML reads and parses the input YAML file.
func readInputYAML(path string) (*KnowledgeBaseInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var kb KnowledgeBaseInput
	if err := yaml.Unmarshal(data, &kb); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &kb, nil
}

// processKnowledgeBase processes FAQs and keywords with incremental updates.
// Implements Requirements 8.5, 8.6
func processKnowledgeBase(ctx context.Context, kb *KnowledgeBaseInput, store *kbstore.SQLiteStore, embClient embedding.Client) (*ProcessStats, error) {
	stats := &ProcessStats{
		TotalFAQs:     len(kb.FAQ),
		TotalKeywords: len(kb.Keywords),
	}

	// Get existing FAQs for incremental update check
	existingFAQs, err := store.GetAllFAQs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing FAQs: %w", err)
	}

	// Build map of existing FAQ hashes for change detection
	existingHashes := make(map[string]string)
	for _, faq := range existingFAQs {
		existingHashes[faq.FAQID] = computeFAQHash(faq.Question, faq.Answer)
	}

	// Process keywords first
	fmt.Println("\nProcessing keywords...")
	if err := processKeywords(ctx, kb.Keywords, store, stats); err != nil {
		return nil, err
	}

	// Process FAQs with progress display
	// Implements Requirement 8.6
	fmt.Println("\nProcessing FAQs...")
	for i, faq := range kb.FAQ {
		// Progress display
		progress := float64(i+1) / float64(len(kb.FAQ)) * 100
		fmt.Printf("\r  [%3.0f%%] Processing %d/%d: %s", progress, i+1, len(kb.FAQ), truncateString(faq.FAQID, 30))

		// Check if FAQ needs update (incremental)
		// Implements Requirement 8.5
		newHash := computeFAQHash(faq.Question, faq.Answer)
		if existingHash, exists := existingHashes[faq.FAQID]; exists {
			if existingHash == newHash {
				// FAQ unchanged, skip
				stats.SkippedFAQs++
				continue
			}
			stats.UpdatedFAQs++
		} else {
			stats.NewFAQs++
		}

		// Generate embedding
		vector, err := embClient.Embed(ctx, faq.Question)
		if err != nil {
			fmt.Printf("\n  ⚠ Error embedding FAQ %s: %v\n", faq.FAQID, err)
			stats.Errors++
			continue
		}

		// Store FAQ with vector
		if err := store.AddFAQ(ctx, faq.FAQID, faq.Question, faq.Answer, vector); err != nil {
			fmt.Printf("\n  ⚠ Error storing FAQ %s: %v\n", faq.FAQID, err)
			stats.Errors++
			continue
		}
	}
	fmt.Println() // New line after progress

	return stats, nil
}

// processKeywords processes keyword entries.
func processKeywords(ctx context.Context, keywords []kbstore.KeywordEntry, store *kbstore.SQLiteStore, stats *ProcessStats) error {
	// Get existing keywords
	existingKWs, err := store.GetAllKeywords(ctx)
	if err != nil {
		return fmt.Errorf("failed to get existing keywords: %w", err)
	}

	existingMap := make(map[string]string)
	for _, kw := range existingKWs {
		existingMap[kw.Keyword] = kw.Reply
	}

	for _, kw := range keywords {
		if existingReply, exists := existingMap[kw.Keyword]; exists {
			if existingReply == kw.Reply {
				// Unchanged, skip
				continue
			}
			stats.UpdatedKWs++
		} else {
			stats.NewKeywords++
		}

		if err := store.AddKeyword(ctx, kw.Keyword, kw.Reply); err != nil {
			fmt.Printf("  ⚠ Error adding keyword %s: %v\n", kw.Keyword, err)
			stats.Errors++
		}
	}

	return nil
}

// computeFAQHash computes a hash of FAQ content for change detection.
func computeFAQHash(question, answer string) string {
	h := sha256.New()
	h.Write([]byte(question))
	h.Write([]byte("|"))
	h.Write([]byte(answer))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// verifyDatabase verifies the integrity of the database.
// Implements Requirement 8.10
func verifyDatabase(dbPath string) error {
	ctx := context.Background()

	// Check if database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", dbPath)
	}

	// Open database
	store, err := kbstore.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer store.Close()

	fmt.Printf("Verifying database: %s\n", dbPath)

	// Verify keywords
	keywords, err := store.GetAllKeywords(ctx)
	if err != nil {
		return fmt.Errorf("failed to read keywords: %w", err)
	}
	fmt.Printf("  Keywords: %d entries\n", len(keywords))

	// Verify FAQs
	faqs, err := store.GetAllFAQs(ctx)
	if err != nil {
		return fmt.Errorf("failed to read FAQs: %w", err)
	}
	fmt.Printf("  FAQs: %d entries\n", len(faqs))

	// Verify vectors
	vectors, err := store.GetAllFAQVectors(ctx)
	if err != nil {
		return fmt.Errorf("failed to read vectors: %w", err)
	}
	fmt.Printf("  Vectors: %d entries\n", len(vectors))

	// Check FAQ-vector consistency
	faqIDs := make(map[string]bool)
	for _, faq := range faqs {
		faqIDs[faq.FAQID] = true
	}

	vectorIDs := make(map[string]bool)
	for _, v := range vectors {
		vectorIDs[v.FAQID] = true
	}

	// Check for FAQs without vectors
	missingVectors := 0
	for faqID := range faqIDs {
		if !vectorIDs[faqID] {
			fmt.Printf("  ⚠ FAQ without vector: %s\n", faqID)
			missingVectors++
		}
	}

	// Check for orphan vectors
	orphanVectors := 0
	for faqID := range vectorIDs {
		if !faqIDs[faqID] {
			fmt.Printf("  ⚠ Orphan vector (no FAQ): %s\n", faqID)
			orphanVectors++
		}
	}

	// Verify vector dimensions are consistent
	if len(vectors) > 0 {
		expectedDim := len(vectors[0].Vector)
		inconsistentDims := 0
		for _, v := range vectors {
			if len(v.Vector) != expectedDim {
				fmt.Printf("  ⚠ Inconsistent vector dimension for %s: expected %d, got %d\n",
					v.FAQID, expectedDim, len(v.Vector))
				inconsistentDims++
			}
		}
		fmt.Printf("  Vector dimension: %d\n", expectedDim)
		if inconsistentDims > 0 {
			return fmt.Errorf("found %d vectors with inconsistent dimensions", inconsistentDims)
		}
	}

	if missingVectors > 0 {
		return fmt.Errorf("found %d FAQs without vectors", missingVectors)
	}

	if orphanVectors > 0 {
		return fmt.Errorf("found %d orphan vectors", orphanVectors)
	}

	return nil
}

// printStats prints the processing statistics.
// Implements Requirement 8.6
func printStats(stats *ProcessStats) {
	fmt.Println("\n=== Processing Summary ===")
	fmt.Printf("Keywords:\n")
	fmt.Printf("  Total: %d\n", stats.TotalKeywords)
	fmt.Printf("  New: %d\n", stats.NewKeywords)
	fmt.Printf("  Updated: %d\n", stats.UpdatedKWs)

	fmt.Printf("FAQs:\n")
	fmt.Printf("  Total: %d\n", stats.TotalFAQs)
	fmt.Printf("  New: %d\n", stats.NewFAQs)
	fmt.Printf("  Updated: %d\n", stats.UpdatedFAQs)
	fmt.Printf("  Skipped (unchanged): %d\n", stats.SkippedFAQs)

	if stats.Errors > 0 {
		fmt.Printf("Errors: %d\n", stats.Errors)
	}

	fmt.Println("==========================")
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s + strings.Repeat(" ", maxLen-len(s))
	}
	return s[:maxLen-3] + "..."
}
