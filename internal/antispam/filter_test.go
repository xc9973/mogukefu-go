// Package antispam provides spam and advertisement filtering.
package antispam

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestIsWhitelistedURL(t *testing.T) {
	cfg := Config{
		WhitelistedDomains: []string{"example.com"},
	}
	f := NewFilter(cfg)

	testCases := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "whitelisted domain",
			url:      "http://example.com/page",
			expected: true,
		},
		{
			name:     "subdomain of whitelisted domain",
			url:      "https://sub.example.com",
			expected: true,
		},
		{
			name:     "https whitelisted domain",
			url:      "https://example.com",
			expected: true,
		},
		{
			name:     "whitelisted domain with path",
			url:      "https://example.com/path/to/something",
			expected: true,
		},
		{
			name:     "non-whitelisted domain",
			url:      "http://google.com",
			expected: false,
		},
		{
			name:     "imposter domain",
			url:      "http://evil-example.com",
			expected: false,
		},
		{
			name:     "imposter domain with whitelisted as subdomain",
			url:      "http://example.com.evil.com",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := f.isWhitelistedURL(tc.url)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestHasExternalLinks(t *testing.T) {
	cfg := Config{
		WhitelistedDomains: []string{"example.com"},
	}
	f := NewFilter(cfg)

	testCases := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "no link",
			text:     "this is a message with no links",
			expected: false,
		},
		{
			name:     "http link",
			text:     "check out http://google.com",
			expected: true,
		},
		{
			name:     "https link",
			text:     "check out https://google.com",
			expected: true,
		},
		{
			name:     "www link",
			text:     "check out www.google.com",
			expected: true,
		},
		{
			name:     "whitelisted link",
			text:     "check out https://example.com",
			expected: false,
		},
		{
			name:     "whitelisted www link",
			text:     "check out www.example.com/page",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &tgbotapi.Message{Text: tc.text}
			actual := f.hasExternalLinks(msg, msg.Text, msg.Text)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
