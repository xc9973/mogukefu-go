// Package config provides configuration management for the bot.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genValidConfig generates a valid Config for testing.
func genValidConfig() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),  // token
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),  // base_url
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),  // api_key
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),  // model
		gen.Float64Range(0.0, 1.0),                                              // similarity_threshold
		gen.IntRange(1, 100),                                                    // short_message_threshold
	).Map(func(values []interface{}) *Config {
		return &Config{
			Bot: BotConfig{
				Token:    values[0].(string),
				AdminIDs: []int64{},
			},
			Embedding: EmbeddingConfig{
				BaseURL: values[1].(string),
				APIKey:  values[2].(string),
				Model:   values[3].(string),
			},
			Vector: VectorConfig{
				SimilarityThreshold:   values[4].(float64),
				ShortMessageThreshold: values[5].(int),
			},
		}
	})
}

// genWhitespaceString generates strings that are empty or contain only whitespace.
func genWhitespaceString() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),
		gen.Const("   "),
		gen.Const("\t"),
		gen.Const("\n"),
		gen.Const("  \t\n  "),
	)
}

// TestProperty1_ConfigValidation tests Property 1: Configuration Validation
// **Feature: go-telegram-intent-bot, Property 1: Configuration Validation**
// **Validates: Requirements 1.4, 1.5, 1.6, 1.7**
//
// For any configuration with invalid values (empty token, missing embedding fields,
// threshold outside 0.0-1.0, non-positive short message threshold),
// the Config_Store SHALL reject the configuration with an appropriate error.
func TestProperty1_ConfigValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 1a: Valid configurations should pass validation
	properties.Property("valid configurations pass validation", prop.ForAll(
		func(cfg *Config) bool {
			err := cfg.Validate()
			return err == nil
		},
		genValidConfig(),
	))

	// Property 1b: Empty or whitespace-only token should fail validation (Requirement 1.4)
	properties.Property("empty token fails validation", prop.ForAll(
		func(emptyToken string, cfg *Config) bool {
			cfg.Bot.Token = emptyToken
			err := cfg.Validate()
			return errors.Is(err, ErrEmptyToken)
		},
		genWhitespaceString(),
		genValidConfig(),
	))

	// Property 1c: Empty or whitespace-only base_url should fail validation (Requirement 1.5)
	properties.Property("empty embedding base_url fails validation", prop.ForAll(
		func(emptyURL string, cfg *Config) bool {
			cfg.Embedding.BaseURL = emptyURL
			err := cfg.Validate()
			return errors.Is(err, ErrMissingEmbeddingBaseURL)
		},
		genWhitespaceString(),
		genValidConfig(),
	))

	// Property 1d: Empty or whitespace-only api_key should fail validation (Requirement 1.5)
	properties.Property("empty embedding api_key fails validation", prop.ForAll(
		func(emptyKey string, cfg *Config) bool {
			cfg.Embedding.APIKey = emptyKey
			err := cfg.Validate()
			return errors.Is(err, ErrMissingEmbeddingAPIKey)
		},
		genWhitespaceString(),
		genValidConfig(),
	))

	// Property 1e: Empty or whitespace-only model should fail validation (Requirement 1.5)
	properties.Property("empty embedding model fails validation", prop.ForAll(
		func(emptyModel string, cfg *Config) bool {
			cfg.Embedding.Model = emptyModel
			err := cfg.Validate()
			return errors.Is(err, ErrMissingEmbeddingModel)
		},
		genWhitespaceString(),
		genValidConfig(),
	))

	// Property 1f: Similarity threshold outside [0.0, 1.0] should fail validation (Requirement 1.6)
	properties.Property("similarity threshold < 0 fails validation", prop.ForAll(
		func(threshold float64, cfg *Config) bool {
			cfg.Vector.SimilarityThreshold = threshold
			err := cfg.Validate()
			return errors.Is(err, ErrInvalidSimilarityThreshold)
		},
		gen.Float64Range(-100.0, -0.001),
		genValidConfig(),
	))

	properties.Property("similarity threshold > 1 fails validation", prop.ForAll(
		func(threshold float64, cfg *Config) bool {
			cfg.Vector.SimilarityThreshold = threshold
			err := cfg.Validate()
			return errors.Is(err, ErrInvalidSimilarityThreshold)
		},
		gen.Float64Range(1.001, 100.0),
		genValidConfig(),
	))

	// Property 1g: Non-positive short message threshold should fail validation (Requirement 1.7)
	properties.Property("non-positive short message threshold fails validation", prop.ForAll(
		func(threshold int, cfg *Config) bool {
			cfg.Vector.ShortMessageThreshold = threshold
			err := cfg.Validate()
			return errors.Is(err, ErrInvalidShortMsgThreshold)
		},
		gen.IntRange(-100, 0),
		genValidConfig(),
	))

	properties.TestingRun(t)
}

// TestLoadConfigFileNotFound tests that Load returns an error for non-existent files (Requirement 1.2)
func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error message, got: %v", err)
	}
}

// TestLoadConfigInvalidFormat tests that Load returns an error for invalid YAML (Requirement 1.3)
func TestLoadConfigInvalidFormat(t *testing.T) {
	// Create a temporary file with invalid YAML
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	
	invalidYAML := `
bot:
  token: "test"
  invalid yaml here: [unclosed bracket
`
	if err := os.WriteFile(tmpFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	_, err := Load(tmpFile)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "format error") {
		t.Errorf("expected 'format error' in error message, got: %v", err)
	}
}

// TestLoadConfigValidFile tests that Load successfully loads a valid config file
func TestLoadConfigValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "valid.yaml")
	
	validYAML := `
bot:
  token: "test-token"
  admin_ids:
    - 123456789

embedding:
  base_url: "https://api.example.com"
  api_key: "test-key"
  model: "test-model"

vector:
  similarity_threshold: 0.75
  short_message_threshold: 10
`
	if err := os.WriteFile(tmpFile, []byte(validYAML), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Bot.Token != "test-token" {
		t.Errorf("expected token 'test-token', got '%s'", cfg.Bot.Token)
	}
	if cfg.Embedding.BaseURL != "https://api.example.com" {
		t.Errorf("expected base_url 'https://api.example.com', got '%s'", cfg.Embedding.BaseURL)
	}
	if cfg.Vector.SimilarityThreshold != 0.75 {
		t.Errorf("expected similarity_threshold 0.75, got %f", cfg.Vector.SimilarityThreshold)
	}
}
