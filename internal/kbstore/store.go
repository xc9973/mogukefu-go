// Package kbstore provides knowledge base storage using SQLite.
package kbstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"gopkg.in/yaml.v3"
)

// KeywordEntry represents a keyword and its reply.
type KeywordEntry struct {
	Keyword string `yaml:"keyword"`
	Reply   string `yaml:"reply"`
}

// FAQEntry represents a FAQ item.
type FAQEntry struct {
	FAQID    string `yaml:"faq_id"`
	Question string `yaml:"question"`
	Answer   string `yaml:"answer"`
}

// FAQVector represents a FAQ with its vector.
type FAQVector struct {
	FAQID  string
	Vector []float64
}

// KnowledgeBase represents the YAML structure for import/export.
type KnowledgeBase struct {
	Keywords []KeywordEntry `yaml:"keywords"`
	FAQ      []FAQEntry     `yaml:"faq"`
}

// Store defines the interface for knowledge base storage.
type Store interface {
	// Initialize creates database tables.
	Initialize(ctx context.Context) error

	// Keyword operations
	AddKeyword(ctx context.Context, keyword, reply string) error
	DeleteKeyword(ctx context.Context, keyword string) error
	GetAllKeywords(ctx context.Context) ([]KeywordEntry, error)

	// FAQ operations
	AddFAQ(ctx context.Context, faqID, question, answer string, vector []float64) error
	DeleteFAQ(ctx context.Context, faqID string) error
	GetFAQ(ctx context.Context, faqID string) (*FAQEntry, error)
	GetAllFAQs(ctx context.Context) ([]FAQEntry, error)
	GetAllFAQVectors(ctx context.Context) ([]FAQVector, error)

	// Import/Export
	ExportToYAML(ctx context.Context) (string, error)
	ImportFromYAML(ctx context.Context, yamlContent string, mode string) error

	// Close closes the database connection.
	Close() error
}

// SQLiteStore implements Store using SQLite with memory caching.
type SQLiteStore struct {
	db *sql.DB

	// Memory cache
	cacheMu      sync.RWMutex
	keywordCache []KeywordEntry
	faqCache     []FAQEntry
	vectorCache  []FAQVector
	cacheLoaded  bool
}

// Open opens a SQLite database connection and returns a new SQLiteStore.
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Initialize creates database tables if they don't exist.
// Validates: Requirements 6.1, 6.2
func (s *SQLiteStore) Initialize(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS keywords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT UNIQUE NOT NULL,
			reply TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS faqs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			faq_id TEXT UNIQUE NOT NULL,
			question TEXT NOT NULL,
			answer TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS faq_vectors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			faq_id TEXT UNIQUE NOT NULL,
			vector BLOB NOT NULL,
			dimension INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (faq_id) REFERENCES faqs(faq_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_keywords_keyword ON keywords(keyword)`,
		`CREATE INDEX IF NOT EXISTS idx_faqs_faq_id ON faqs(faq_id)`,
		`CREATE INDEX IF NOT EXISTS idx_faq_vectors_faq_id ON faq_vectors(faq_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// AddKeyword adds a new keyword or updates an existing one.
// Validates: Requirements 6.3
func (s *SQLiteStore) AddKeyword(ctx context.Context, keyword, reply string) error {
	query := `INSERT INTO keywords (keyword, reply, updated_at) 
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(keyword) DO UPDATE SET reply = excluded.reply, updated_at = CURRENT_TIMESTAMP`

	if _, err := s.db.ExecContext(ctx, query, keyword, reply); err != nil {
		return fmt.Errorf("failed to add keyword: %w", err)
	}

	s.invalidateCache()
	return nil
}

// DeleteKeyword deletes a keyword.
// Validates: Requirements 6.3
func (s *SQLiteStore) DeleteKeyword(ctx context.Context, keyword string) error {
	query := `DELETE FROM keywords WHERE keyword = ?`

	result, err := s.db.ExecContext(ctx, query, keyword)
	if err != nil {
		return fmt.Errorf("failed to delete keyword: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("keyword not found: %s", keyword)
	}

	s.invalidateCache()
	return nil
}

// GetAllKeywords returns all keywords from cache or database.
// Validates: Requirements 6.3, 6.5
func (s *SQLiteStore) GetAllKeywords(ctx context.Context) ([]KeywordEntry, error) {
	s.cacheMu.RLock()
	if s.cacheLoaded && s.keywordCache != nil {
		result := make([]KeywordEntry, len(s.keywordCache))
		copy(result, s.keywordCache)
		s.cacheMu.RUnlock()
		return result, nil
	}
	s.cacheMu.RUnlock()

	return s.loadKeywordsFromDB(ctx)
}

func (s *SQLiteStore) loadKeywordsFromDB(ctx context.Context) ([]KeywordEntry, error) {
	query := `SELECT keyword, reply FROM keywords ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query keywords: %w", err)
	}
	defer rows.Close()

	var keywords []KeywordEntry
	for rows.Next() {
		var entry KeywordEntry
		if err := rows.Scan(&entry.Keyword, &entry.Reply); err != nil {
			return nil, fmt.Errorf("failed to scan keyword: %w", err)
		}
		keywords = append(keywords, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating keywords: %w", err)
	}

	// Update cache
	s.cacheMu.Lock()
	s.keywordCache = keywords
	s.cacheMu.Unlock()

	return keywords, nil
}

// AddFAQ adds a new FAQ with its vector or updates an existing one.
// Validates: Requirements 6.4
func (s *SQLiteStore) AddFAQ(ctx context.Context, faqID, question, answer string, vector []float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or update FAQ
	faqQuery := `INSERT INTO faqs (faq_id, question, answer, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(faq_id) DO UPDATE SET 
			question = excluded.question, 
			answer = excluded.answer, 
			updated_at = CURRENT_TIMESTAMP`

	if _, err := tx.ExecContext(ctx, faqQuery, faqID, question, answer); err != nil {
		return fmt.Errorf("failed to add FAQ: %w", err)
	}

	// Insert or update vector
	if len(vector) > 0 {
		vectorBlob, err := encodeVector(vector)
		if err != nil {
			return fmt.Errorf("failed to encode vector: %w", err)
		}

		vectorQuery := `INSERT INTO faq_vectors (faq_id, vector, dimension, created_at) 
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(faq_id) DO UPDATE SET 
				vector = excluded.vector, 
				dimension = excluded.dimension`

		if _, err := tx.ExecContext(ctx, vectorQuery, faqID, vectorBlob, len(vector)); err != nil {
			return fmt.Errorf("failed to add FAQ vector: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.invalidateCache()
	return nil
}

// DeleteFAQ deletes a FAQ and its vector (cascade).
// Validates: Requirements 6.4
func (s *SQLiteStore) DeleteFAQ(ctx context.Context, faqID string) error {
	query := `DELETE FROM faqs WHERE faq_id = ?`

	result, err := s.db.ExecContext(ctx, query, faqID)
	if err != nil {
		return fmt.Errorf("failed to delete FAQ: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("FAQ not found: %s", faqID)
	}

	s.invalidateCache()
	return nil
}

// GetFAQ returns a single FAQ by ID.
// Validates: Requirements 6.4
func (s *SQLiteStore) GetFAQ(ctx context.Context, faqID string) (*FAQEntry, error) {
	query := `SELECT faq_id, question, answer FROM faqs WHERE faq_id = ?`

	var entry FAQEntry
	err := s.db.QueryRowContext(ctx, query, faqID).Scan(&entry.FAQID, &entry.Question, &entry.Answer)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get FAQ: %w", err)
	}

	return &entry, nil
}

// GetAllFAQs returns all FAQs from cache or database.
// Validates: Requirements 6.4, 6.5
func (s *SQLiteStore) GetAllFAQs(ctx context.Context) ([]FAQEntry, error) {
	s.cacheMu.RLock()
	if s.cacheLoaded && s.faqCache != nil {
		result := make([]FAQEntry, len(s.faqCache))
		copy(result, s.faqCache)
		s.cacheMu.RUnlock()
		return result, nil
	}
	s.cacheMu.RUnlock()

	return s.loadFAQsFromDB(ctx)
}

func (s *SQLiteStore) loadFAQsFromDB(ctx context.Context) ([]FAQEntry, error) {
	query := `SELECT faq_id, question, answer FROM faqs ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query FAQs: %w", err)
	}
	defer rows.Close()

	var faqs []FAQEntry
	for rows.Next() {
		var entry FAQEntry
		if err := rows.Scan(&entry.FAQID, &entry.Question, &entry.Answer); err != nil {
			return nil, fmt.Errorf("failed to scan FAQ: %w", err)
		}
		faqs = append(faqs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating FAQs: %w", err)
	}

	// Update cache
	s.cacheMu.Lock()
	s.faqCache = faqs
	s.cacheMu.Unlock()

	return faqs, nil
}

// GetAllFAQVectors returns all FAQ vectors from cache or database.
// Validates: Requirements 6.4, 6.5
func (s *SQLiteStore) GetAllFAQVectors(ctx context.Context) ([]FAQVector, error) {
	s.cacheMu.RLock()
	if s.cacheLoaded && s.vectorCache != nil {
		result := make([]FAQVector, len(s.vectorCache))
		copy(result, s.vectorCache)
		s.cacheMu.RUnlock()
		return result, nil
	}
	s.cacheMu.RUnlock()

	return s.loadVectorsFromDB(ctx)
}

func (s *SQLiteStore) loadVectorsFromDB(ctx context.Context) ([]FAQVector, error) {
	query := `SELECT faq_id, vector FROM faq_vectors ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query FAQ vectors: %w", err)
	}
	defer rows.Close()

	var vectors []FAQVector
	for rows.Next() {
		var faqID string
		var vectorBlob []byte
		if err := rows.Scan(&faqID, &vectorBlob); err != nil {
			return nil, fmt.Errorf("failed to scan FAQ vector: %w", err)
		}

		vector, err := decodeVector(vectorBlob)
		if err != nil {
			return nil, fmt.Errorf("failed to decode vector for FAQ %s: %w", faqID, err)
		}

		vectors = append(vectors, FAQVector{
			FAQID:  faqID,
			Vector: vector,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating FAQ vectors: %w", err)
	}

	// Update cache
	s.cacheMu.Lock()
	s.vectorCache = vectors
	s.cacheMu.Unlock()

	return vectors, nil
}

// ExportToYAML exports all keywords and FAQs to YAML format.
// Validates: Requirements 6.7
func (s *SQLiteStore) ExportToYAML(ctx context.Context) (string, error) {
	keywords, err := s.GetAllKeywords(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get keywords: %w", err)
	}

	faqs, err := s.GetAllFAQs(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get FAQs: %w", err)
	}

	kb := KnowledgeBase{
		Keywords: keywords,
		FAQ:      faqs,
	}

	data, err := yaml.Marshal(&kb)
	if err != nil {
		return "", fmt.Errorf("failed to marshal YAML: %w", err)
	}

	return string(data), nil
}

// ImportFromYAML imports keywords and FAQs from YAML content.
// mode can be "merge" (add/update) or "overwrite" (replace all).
// Note: This does not import vectors - use CLI tool for that.
// Validates: Requirements 6.8
func (s *SQLiteStore) ImportFromYAML(ctx context.Context, yamlContent string, mode string) error {
	var kb KnowledgeBase
	if err := yaml.Unmarshal([]byte(yamlContent), &kb); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// If overwrite mode, delete all existing data
	if mode == "overwrite" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM keywords"); err != nil {
			return fmt.Errorf("failed to delete keywords: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM faqs"); err != nil {
			return fmt.Errorf("failed to delete FAQs: %w", err)
		}
	}

	// Import keywords
	keywordQuery := `INSERT INTO keywords (keyword, reply, updated_at) 
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(keyword) DO UPDATE SET reply = excluded.reply, updated_at = CURRENT_TIMESTAMP`

	for _, kw := range kb.Keywords {
		if _, err := tx.ExecContext(ctx, keywordQuery, kw.Keyword, kw.Reply); err != nil {
			return fmt.Errorf("failed to import keyword %s: %w", kw.Keyword, err)
		}
	}

	// Import FAQs (without vectors)
	faqQuery := `INSERT INTO faqs (faq_id, question, answer, updated_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(faq_id) DO UPDATE SET 
			question = excluded.question, 
			answer = excluded.answer, 
			updated_at = CURRENT_TIMESTAMP`

	for _, faq := range kb.FAQ {
		if _, err := tx.ExecContext(ctx, faqQuery, faq.FAQID, faq.Question, faq.Answer); err != nil {
			return fmt.Errorf("failed to import FAQ %s: %w", faq.FAQID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.invalidateCache()
	return nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// invalidateCache marks the cache as invalid.
// Validates: Requirements 6.6
func (s *SQLiteStore) invalidateCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cacheLoaded = false
	s.keywordCache = nil
	s.faqCache = nil
	s.vectorCache = nil
}

// RefreshCache reloads all data from database into cache.
// Validates: Requirements 6.5, 6.6
func (s *SQLiteStore) RefreshCache(ctx context.Context) error {
	// Load all data
	if _, err := s.loadKeywordsFromDB(ctx); err != nil {
		return err
	}
	if _, err := s.loadFAQsFromDB(ctx); err != nil {
		return err
	}
	if _, err := s.loadVectorsFromDB(ctx); err != nil {
		return err
	}

	s.cacheMu.Lock()
	s.cacheLoaded = true
	s.cacheMu.Unlock()

	return nil
}

// encodeVector encodes a float64 slice to bytes using little-endian binary format.
func encodeVector(vector []float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, v := range vector {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// decodeVector decodes bytes to a float64 slice using little-endian binary format.
func decodeVector(data []byte) ([]float64, error) {
	if len(data)%8 != 0 {
		return nil, fmt.Errorf("invalid vector data length: %d", len(data))
	}

	count := len(data) / 8
	vector := make([]float64, count)
	buf := bytes.NewReader(data)

	for i := 0; i < count; i++ {
		if err := binary.Read(buf, binary.LittleEndian, &vector[i]); err != nil {
			return nil, err
		}
	}

	return vector, nil
}
