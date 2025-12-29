// Package config provides configuration management for the bot.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validation errors
var (
	ErrEmptyToken                = errors.New("bot token cannot be empty")
	ErrMissingEmbeddingBaseURL   = errors.New("embedding base_url is required")
	ErrMissingEmbeddingAPIKey    = errors.New("embedding api_key is required")
	ErrMissingEmbeddingModel     = errors.New("embedding model is required")
	ErrInvalidSimilarityThreshold = errors.New("similarity_threshold must be between 0.0 and 1.0")
	ErrInvalidShortMsgThreshold  = errors.New("short_message_threshold must be a positive integer")
)

// Config holds all configuration for the bot.
type Config struct {
	Bot       BotConfig       `yaml:"bot"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Vector    VectorConfig    `yaml:"vector"`
}

// Load loads configuration from a YAML file.
// Returns an error if the file doesn't exist, has invalid format, or fails validation.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Extract line/column info from yaml error if available
		var yamlErr *yaml.TypeError
		if errors.As(err, &yamlErr) {
			return nil, fmt.Errorf("config file format error: %s", strings.Join(yamlErr.Errors, "; "))
		}
		return nil, fmt.Errorf("config file format error at parsing: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration.
// Returns an error if any required field is missing or has an invalid value.
func (c *Config) Validate() error {
	// Validate Bot Token (Requirement 1.4)
	if strings.TrimSpace(c.Bot.Token) == "" {
		return ErrEmptyToken
	}

	// Validate Embedding config (Requirement 1.5)
	if strings.TrimSpace(c.Embedding.BaseURL) == "" {
		return ErrMissingEmbeddingBaseURL
	}
	if strings.TrimSpace(c.Embedding.APIKey) == "" {
		return ErrMissingEmbeddingAPIKey
	}
	if strings.TrimSpace(c.Embedding.Model) == "" {
		return ErrMissingEmbeddingModel
	}

	// Validate similarity threshold (Requirement 1.6)
	if c.Vector.SimilarityThreshold < 0.0 || c.Vector.SimilarityThreshold > 1.0 {
		return ErrInvalidSimilarityThreshold
	}

	// Validate short message threshold (Requirement 1.7)
	if c.Vector.ShortMessageThreshold <= 0 {
		return ErrInvalidShortMsgThreshold
	}

	return nil
}

// BotConfig holds Telegram bot configuration.
type BotConfig struct {
	Token    string  `yaml:"token"`
	AdminIDs []int64 `yaml:"admin_ids"`
}

// EmbeddingConfig holds embedding API configuration.
type EmbeddingConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

// VectorConfig holds vector search configuration.
type VectorConfig struct {
	SimilarityThreshold   float64 `yaml:"similarity_threshold"`
	ShortMessageThreshold int     `yaml:"short_message_threshold"`
}

// SetDefaults sets default values for optional configuration fields.
func (c *Config) SetDefaults() {
	if c.Vector.SimilarityThreshold == 0 {
		c.Vector.SimilarityThreshold = 0.75
	}
	if c.Vector.ShortMessageThreshold == 0 {
		c.Vector.ShortMessageThreshold = 10
	}
}
