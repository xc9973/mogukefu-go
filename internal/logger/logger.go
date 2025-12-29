// Package logger provides structured logging functionality using log/slog.
// Implements Requirements 9.1, 9.2, 9.3, 9.4, 9.5, 9.6
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents log levels.
type Level = slog.Level

// Log level constants.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config holds logger configuration.
type Config struct {
	// Level is the minimum log level to output.
	Level Level
	// Output is the writer to output logs to. Defaults to os.Stdout.
	Output io.Writer
	// Format is the output format: "text" or "json". Defaults to "text".
	Format string
}

// ParseLevel parses a log level string into a Level.
// Supports: debug, info, warn, error (case-insensitive).
// Returns LevelInfo for unknown values.
// Implements Requirements 9.6
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// New creates a new structured logger with the given configuration.
// Implements Requirements 9.6
func New(cfg Config) *slog.Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: cfg.Level,
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	return slog.New(handler)
}

// Default creates a default logger with info level and text format.
func Default() *slog.Logger {
	return New(Config{
		Level:  LevelInfo,
		Output: os.Stdout,
		Format: "text",
	})
}


// MessageAttrs creates log attributes for a received message.
// Implements Requirements 9.1
func MessageAttrs(chatID int64, topicID int, fromID int64, textLen int) []any {
	attrs := []any{
		"chat_id", chatID,
		"from_id", fromID,
		"text_length", textLen,
	}
	if topicID > 0 {
		attrs = append(attrs, "topic_id", topicID)
	}
	return attrs
}

// KeywordMatchAttrs creates log attributes for a keyword match result.
// Implements Requirements 9.2
func KeywordMatchAttrs(keyword string, chatID int64) []any {
	return []any{
		"keyword", keyword,
		"chat_id", chatID,
	}
}

// FAQMatchAttrs creates log attributes for a FAQ match result.
// Implements Requirements 9.3
func FAQMatchAttrs(faqID string, similarity float64, chatID int64) []any {
	return []any{
		"faq_id", faqID,
		"similarity", similarity,
		"chat_id", chatID,
	}
}

// ReplySentAttrs creates log attributes for a sent reply.
// Implements Requirements 9.4
func ReplySentAttrs(chatID int64, messageID int) []any {
	return []any{
		"chat_id", chatID,
		"message_id", messageID,
	}
}

// ErrorAttrs creates log attributes for an error.
// Implements Requirements 9.5
func ErrorAttrs(err error, context ...any) []any {
	attrs := []any{"error", err.Error()}
	attrs = append(attrs, context...)
	return attrs
}

// CommandAttrs creates log attributes for a received command.
func CommandAttrs(command string, args string, fromID int64, chatID int64) []any {
	return []any{
		"command", command,
		"args", args,
		"from_id", fromID,
		"chat_id", chatID,
	}
}

// VectorSearchAttrs creates log attributes for vector search operations.
// Implements Requirements 9.3
func VectorSearchAttrs(queryLen int, threshold float64, resultCount int) []any {
	return []any{
		"query_length", queryLen,
		"threshold", threshold,
		"result_count", resultCount,
	}
}

// EmbeddingAttrs creates log attributes for embedding operations.
func EmbeddingAttrs(textCount int, model string, duration string) []any {
	return []any{
		"text_count", textCount,
		"model", model,
		"duration", duration,
	}
}
