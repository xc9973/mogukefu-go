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

func TestCheckChannelSpam(t *testing.T) {
	cfg := Config{
		BlockForwardedChannels: true,
		WhitelistedChannels:    []int64{12345},
	}
	f := NewFilter(cfg)

	nonWhitelistedChannelChat := &tgbotapi.Chat{ID: 67890, Type: "channel"}
	whitelistedChannelChat := &tgbotapi.Chat{ID: 12345, Type: "channel"}

	testCases := []struct {
		name     string
		message  *tgbotapi.Message
		isSpam   bool
		reason   string
	}{
		{
			name:   "reply to forwarded message from non-whitelisted channel",
			message: &tgbotapi.Message{
				Text: "spam reply",
				ReplyToMessage: &tgbotapi.Message{
					ForwardFromChat: nonWhitelistedChannelChat,
				},
			},
			isSpam: true,
			reason: "回复了频道消息（疑似广告引流）",
		},
		{
			name:   "reply to message from non-whitelisted channel sender",
			message: &tgbotapi.Message{
				Text: "spam reply",
				ReplyToMessage: &tgbotapi.Message{
					SenderChat: nonWhitelistedChannelChat,
				},
			},
			isSpam: true,
			reason: "回复了频道消息（疑似广告引流）",
		},
		{
			name:   "reply to forwarded message from whitelisted channel",
			message: &tgbotapi.Message{
				Text: "not spam",
				ReplyToMessage: &tgbotapi.Message{
					ForwardFromChat: whitelistedChannelChat,
				},
			},
			isSpam: false,
		},
		{
			name:   "reply to message from whitelisted channel sender",
			message: &tgbotapi.Message{
				Text: "not spam",
				ReplyToMessage: &tgbotapi.Message{
					SenderChat: whitelistedChannelChat,
				},
			},
			isSpam: false,
		},
		{
			name: "forward from non-whitelisted channel",
			message: &tgbotapi.Message{
				ForwardFromChat: nonWhitelistedChannelChat,
			},
			isSpam: true,
			reason: "转发自其他频道的消息",
		},
		{
			name: "message from non-whitelisted channel sender",
			message: &tgbotapi.Message{
				SenderChat: nonWhitelistedChannelChat,
			},
			isSpam: true,
			reason: "以频道身份发送的消息",
		},
		{
			name: "forward of user reply to channel message",
			message: &tgbotapi.Message{
				ForwardFromChat: &tgbotapi.Chat{ID: 99999, Type: "supergroup"}, // forwarded from a group
				ReplyToMessage: &tgbotapi.Message{
					ForwardFromChat: nonWhitelistedChannelChat, // original reply was to a channel message
				},
			},
			isSpam: true,
			reason: "回复了频道消息（疑似广告引流）",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := f.Check(tc.message)
			if result.IsSpam != tc.isSpam {
				t.Errorf("expected IsSpam %v, got %v", tc.isSpam, result.IsSpam)
			}
			if result.IsSpam && result.Reason != tc.reason {
				t.Errorf("expected Reason %q, got %q", tc.reason, result.Reason)
			}
		})
	}
}
