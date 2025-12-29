// Package logger provides structured logging functionality using log/slog.
package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"Debug", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"WARN", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"unknown", LevelInfo}, // defaults to info
		{"", LevelInfo},        // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("text format", func(t *testing.T) {
		var buf bytes.Buffer
		log := New(Config{
			Level:  LevelInfo,
			Output: &buf,
			Format: "text",
		})

		log.Info("test message", "key", "value")

		output := buf.String()
		if !strings.Contains(output, "test message") {
			t.Errorf("expected output to contain 'test message', got: %s", output)
		}
		if !strings.Contains(output, "key=value") {
			t.Errorf("expected output to contain 'key=value', got: %s", output)
		}
	})

	t.Run("json format", func(t *testing.T) {
		var buf bytes.Buffer
		log := New(Config{
			Level:  LevelInfo,
			Output: &buf,
			Format: "json",
		})

		log.Info("test message", "key", "value")

		output := buf.String()
		if !strings.Contains(output, `"msg":"test message"`) {
			t.Errorf("expected JSON output to contain message, got: %s", output)
		}
		if !strings.Contains(output, `"key":"value"`) {
			t.Errorf("expected JSON output to contain key-value, got: %s", output)
		}
	})

	t.Run("respects log level", func(t *testing.T) {
		var buf bytes.Buffer
		log := New(Config{
			Level:  LevelWarn,
			Output: &buf,
			Format: "text",
		})

		log.Info("info message")
		log.Warn("warn message")

		output := buf.String()
		if strings.Contains(output, "info message") {
			t.Errorf("expected info message to be filtered out, got: %s", output)
		}
		if !strings.Contains(output, "warn message") {
			t.Errorf("expected warn message to be present, got: %s", output)
		}
	})
}

func TestDefault(t *testing.T) {
	log := Default()
	if log == nil {
		t.Error("Default() returned nil")
	}
}

func TestMessageAttrs(t *testing.T) {
	t.Run("without topic_id", func(t *testing.T) {
		attrs := MessageAttrs(123, 0, 456, 100)
		expected := []any{"chat_id", int64(123), "from_id", int64(456), "text_length", 100}
		if len(attrs) != len(expected) {
			t.Errorf("expected %d attrs, got %d", len(expected), len(attrs))
		}
	})

	t.Run("with topic_id", func(t *testing.T) {
		attrs := MessageAttrs(123, 789, 456, 100)
		// Should have 8 elements (4 key-value pairs)
		if len(attrs) != 8 {
			t.Errorf("expected 8 attrs with topic_id, got %d", len(attrs))
		}
		// Check that topic_id is included
		found := false
		for i := 0; i < len(attrs); i += 2 {
			if attrs[i] == "topic_id" {
				found = true
				if attrs[i+1] != 789 {
					t.Errorf("expected topic_id=789, got %v", attrs[i+1])
				}
			}
		}
		if !found {
			t.Error("topic_id not found in attrs")
		}
	})
}

func TestKeywordMatchAttrs(t *testing.T) {
	attrs := KeywordMatchAttrs("test", 123)
	if len(attrs) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(attrs))
	}
	if attrs[0] != "keyword" || attrs[1] != "test" {
		t.Errorf("expected keyword=test, got %v=%v", attrs[0], attrs[1])
	}
	if attrs[2] != "chat_id" || attrs[3] != int64(123) {
		t.Errorf("expected chat_id=123, got %v=%v", attrs[2], attrs[3])
	}
}

func TestFAQMatchAttrs(t *testing.T) {
	attrs := FAQMatchAttrs("faq1", 0.85, 123)
	if len(attrs) != 6 {
		t.Errorf("expected 6 attrs, got %d", len(attrs))
	}
	if attrs[0] != "faq_id" || attrs[1] != "faq1" {
		t.Errorf("expected faq_id=faq1, got %v=%v", attrs[0], attrs[1])
	}
	if attrs[2] != "similarity" || attrs[3] != 0.85 {
		t.Errorf("expected similarity=0.85, got %v=%v", attrs[2], attrs[3])
	}
}

func TestReplySentAttrs(t *testing.T) {
	attrs := ReplySentAttrs(123, 456)
	if len(attrs) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(attrs))
	}
}

func TestErrorAttrs(t *testing.T) {
	err := &testError{msg: "test error"}
	attrs := ErrorAttrs(err, "context_key", "context_value")
	if len(attrs) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(attrs))
	}
	if attrs[0] != "error" || attrs[1] != "test error" {
		t.Errorf("expected error=test error, got %v=%v", attrs[0], attrs[1])
	}
}

func TestCommandAttrs(t *testing.T) {
	attrs := CommandAttrs("addkw", "test reply", 123, 456)
	if len(attrs) != 8 {
		t.Errorf("expected 8 attrs, got %d", len(attrs))
	}
}

func TestVectorSearchAttrs(t *testing.T) {
	attrs := VectorSearchAttrs(100, 0.75, 5)
	if len(attrs) != 6 {
		t.Errorf("expected 6 attrs, got %d", len(attrs))
	}
}

func TestEmbeddingAttrs(t *testing.T) {
	attrs := EmbeddingAttrs(10, "text-embedding-3-small", "100ms")
	if len(attrs) != 6 {
		t.Errorf("expected 6 attrs, got %d", len(attrs))
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestLoggerIntegration tests that the logger works correctly with slog.
func TestLoggerIntegration(t *testing.T) {
	var buf bytes.Buffer
	log := New(Config{
		Level:  LevelDebug,
		Output: &buf,
		Format: "text",
	})

	// Test all log levels
	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("expected debug message in output")
	}
	if !strings.Contains(output, "info message") {
		t.Error("expected info message in output")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("expected warn message in output")
	}
	if !strings.Contains(output, "error message") {
		t.Error("expected error message in output")
	}
}

// Ensure slog.Logger is returned
func TestNewReturnsSlogLogger(t *testing.T) {
	log := New(Config{Level: LevelInfo})
	var _ *slog.Logger = log // compile-time check
}
