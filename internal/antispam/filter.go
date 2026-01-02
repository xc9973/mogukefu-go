// Package antispam provides spam and advertisement filtering.
package antispam

import (
	"net/url"
	"regexp"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// FilterResult represents the result of spam filtering.
type FilterResult struct {
	IsSpam   bool
	Reason   string
	Action   Action
}

// Action defines what action to take on spam.
type Action int

const (
	ActionNone Action = iota
	ActionDelete
	ActionWarn
	ActionBan
)

// Config holds configuration for the spam filter.
type Config struct {
	// Enable/disable features
	BlockForwardedChannels bool     // Block messages forwarded from channels
	BlockExternalLinks     bool     // Block messages with external links
	BlockKeywords          bool     // Block messages containing spam keywords
	
	// Whitelist
	WhitelistedDomains  []string // Domains that are allowed (e.g., your own domain)
	WhitelistedUsers    []int64  // Users that bypass spam filter
	WhitelistedChannels []int64  // Channel IDs that are allowed to be forwarded from
	
	// Spam keywords
	SpamKeywords []string // Keywords that trigger spam detection
}

// Filter implements spam filtering logic.
type Filter struct {
	config Config
	
	mu               sync.RWMutex
	spamKeywords     []string
	whitelistDomains map[string]bool
	whitelistUsers   map[int64]bool
	whitelistChannels map[int64]bool
	
	// Compiled regex for URL detection
	urlRegex *regexp.Regexp
}

// NewFilter creates a new spam filter.
func NewFilter(cfg Config) *Filter {
	// Pre-lowercase spam keywords
	spamKeywords := make([]string, len(cfg.SpamKeywords))
	for i, kw := range cfg.SpamKeywords {
		spamKeywords[i] = strings.ToLower(kw)
	}

	f := &Filter{
		config:            cfg,
		spamKeywords:      spamKeywords,
		whitelistDomains:  make(map[string]bool),
		whitelistUsers:    make(map[int64]bool),
		whitelistChannels: make(map[int64]bool),
	}

	// Build whitelist maps
	for _, domain := range cfg.WhitelistedDomains {
		f.whitelistDomains[strings.ToLower(domain)] = true
	}
	for _, userID := range cfg.WhitelistedUsers {
		f.whitelistUsers[userID] = true
	}
	for _, channelID := range cfg.WhitelistedChannels {
		f.whitelistChannels[channelID] = true
	}
	
	// Compile URL regex
	// Modified to NOT match @mentions (e.g. @username) as URLs
	// Original: `(?i)(https?://|t\.me/|@)[^\s]+`
	f.urlRegex = regexp.MustCompile(`(?i)(https?://|www\.|t\.me/)[^\s]+`)
	
	return f
}

// Check checks if a message is spam.
func (f *Filter) Check(msg *tgbotapi.Message) *FilterResult {
	// Skip if user is whitelisted
	if msg.From != nil && f.isUserWhitelisted(msg.From.ID) {
		return &FilterResult{IsSpam: false}
	}

	// Check forwarded messages from channels
	isFromWhitelistedChannel := false
	if f.config.BlockForwardedChannels {
		if reason := f.checkChannelSpam(msg); reason != "" {
			return &FilterResult{
				IsSpam: true,
				Reason: reason,
				Action: ActionDelete,
			}
		}
		// Track if message is from whitelisted channel
		isFromWhitelistedChannel = f.isFromWhitelistedChannel(msg)
	}

	// Helper for text-based checks
	// Get message text and compute lowercase version once
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	textLower := strings.ToLower(text)

	// Skip external link check for whitelisted channels
	if !isFromWhitelistedChannel && f.config.BlockExternalLinks && f.hasExternalLinks(msg, text, textLower) {
		return &FilterResult{
			IsSpam: true,
			Reason: "包含外部链接",
			Action: ActionDelete,
		}
	}

	// Skip keyword check for whitelisted channels
	if !isFromWhitelistedChannel && f.config.BlockKeywords && f.hasSpamKeywords(textLower) {
		return &FilterResult{
			IsSpam: true,
			Reason: "包含广告关键词",
			Action: ActionDelete,
		}
	}

	return &FilterResult{IsSpam: false}
}

// isUserWhitelisted checks if a user is whitelisted.
func (f *Filter) isUserWhitelisted(userID int64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.whitelistUsers[userID]
}

// isForwardedFromChannel checks if message is forwarded from a channel.
func (f *Filter) isForwardedFromChannel(msg *tgbotapi.Message) bool {
	return f.checkChannelSpam(msg) != ""
}

// checkChannelSpam checks for channel-related spam and returns the reason if found.
func (f *Filter) checkChannelSpam(msg *tgbotapi.Message) string {
	// Check if message is forwarded from a channel
	if msg.ForwardFromChat != nil {
		if msg.ForwardFromChat.Type == "channel" {
			if !f.isChannelWhitelisted(msg.ForwardFromChat.ID) {
				return "转发自其他频道的消息"
			}
		}
	}
	
	// Check if this message is a reply to a channel message
	// This catches the case where someone replies to a channel post in the group
	if msg.ReplyToMessage != nil {
		// Check if the replied message is forwarded from a channel
		if msg.ReplyToMessage.ForwardFromChat != nil {
			if msg.ReplyToMessage.ForwardFromChat.Type == "channel" {
				if !f.isChannelWhitelisted(msg.ReplyToMessage.ForwardFromChat.ID) {
					return "回复了频道消息（疑似广告引流）"
				}
			}
		}
		// Check if the replied message itself is a channel post (SenderChat)
		if msg.ReplyToMessage.SenderChat != nil {
			if msg.ReplyToMessage.SenderChat.Type == "channel" {
				if !f.isChannelWhitelisted(msg.ReplyToMessage.SenderChat.ID) {
					return "回复了频道消息（疑似广告引流）"
				}
			}
		}
	}
	
	// Check if the message itself is sent by a channel (SenderChat)
	// This happens when a channel posts directly to a linked group
	if msg.SenderChat != nil {
		if msg.SenderChat.Type == "channel" {
			if !f.isChannelWhitelisted(msg.SenderChat.ID) {
				return "以频道身份发送的消息"
			}
		}
	}
	
	// Also check ForwardFrom for forwarded messages from users
	// who have privacy settings that hide their identity
	if msg.ForwardSenderName != "" {
		// This is a forwarded message but we can't see the source
		// You might want to be more lenient here
		return ""
	}
	
	return ""
}

// isChannelWhitelisted checks if a channel is whitelisted.
func (f *Filter) isChannelWhitelisted(channelID int64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.whitelistChannels[channelID]
}

// isFromWhitelistedChannel checks if message is from a whitelisted channel.
func (f *Filter) isFromWhitelistedChannel(msg *tgbotapi.Message) bool {
	if msg.ForwardFromChat != nil && msg.ForwardFromChat.Type == "channel" {
		return f.isChannelWhitelisted(msg.ForwardFromChat.ID)
	}
	if msg.SenderChat != nil && msg.SenderChat.Type == "channel" {
		return f.isChannelWhitelisted(msg.SenderChat.ID)
	}
	return false
}

// hasExternalLinks checks if message contains external links.
func (f *Filter) hasExternalLinks(msg *tgbotapi.Message, text, textLower string) bool {
	if text == "" {
		return false
	}

	// Find all URLs
	matches := f.urlRegex.FindAllString(text, -1)
	for _, match := range matches {
		if !f.isWhitelistedURL(match) {
			return true
		}
	}

	// Check entities for URLs
	entities := msg.Entities
	if entities == nil {
		entities = msg.CaptionEntities
	}
	for _, entity := range entities {
		if entity.Type == "url" || entity.Type == "text_link" {
			url := entity.URL
			if url == "" && entity.Type == "url" {
				// Extract URL from text
				if entity.Offset+entity.Length <= len(text) {
					url = text[entity.Offset : entity.Offset+entity.Length]
				}
			}
			if !f.isWhitelistedURL(url) {
				return true
			}
		}
	}

	return false
}

// isWhitelistedURL checks if a URL is from a whitelisted domain.
func (f *Filter) isWhitelistedURL(rawURL string) bool {
	rawURL = strings.ToLower(rawURL)
	// Add a scheme if missing, to help url.Parse
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, deny.
		return false
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return false
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	for domain := range f.whitelistDomains {
		if hostname == domain || strings.HasSuffix(hostname, "."+domain) {
			return true
		}
	}

	return false
}

// hasSpamKeywords checks if message contains spam keywords.
func (f *Filter) hasSpamKeywords(textLower string) bool {
	if textLower == "" {
		return false
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, keyword := range f.spamKeywords {
		if strings.Contains(textLower, keyword) {
			return true
		}
	}

	return false
}

// UpdateSpamKeywords updates the spam keywords list.
func (f *Filter) UpdateSpamKeywords(keywords []string) {
	lowerKeywords := make([]string, len(keywords))
	for i, kw := range keywords {
		lowerKeywords[i] = strings.ToLower(kw)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spamKeywords = lowerKeywords
}

// AddSpamKeyword adds a spam keyword.
func (f *Filter) AddSpamKeyword(keyword string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spamKeywords = append(f.spamKeywords, strings.ToLower(keyword))
}

// RemoveSpamKeyword removes a spam keyword.
func (f *Filter) RemoveSpamKeyword(keyword string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	keyword = strings.ToLower(keyword)
	for i, kw := range f.spamKeywords {
		if kw == keyword {
			f.spamKeywords = append(f.spamKeywords[:i], f.spamKeywords[i+1:]...)
			return true
		}
	}
	return false
}

// GetSpamKeywords returns the current spam keywords.
func (f *Filter) GetSpamKeywords() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	result := make([]string, len(f.spamKeywords))
	copy(result, f.spamKeywords)
	return result
}

// AddWhitelistedUser adds a user to the whitelist.
func (f *Filter) AddWhitelistedUser(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whitelistUsers[userID] = true
}

// RemoveWhitelistedUser removes a user from the whitelist.
func (f *Filter) RemoveWhitelistedUser(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.whitelistUsers, userID)
}

// AddWhitelistedChannel adds a channel to the whitelist.
func (f *Filter) AddWhitelistedChannel(channelID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whitelistChannels[channelID] = true
}

// RemoveWhitelistedChannel removes a channel from the whitelist.
func (f *Filter) RemoveWhitelistedChannel(channelID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.whitelistChannels[channelID] {
		delete(f.whitelistChannels, channelID)
		return true
	}
	return false
}

// GetWhitelistedChannels returns all whitelisted channel IDs.
func (f *Filter) GetWhitelistedChannels() []int64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	result := make([]int64, 0, len(f.whitelistChannels))
	for id := range f.whitelistChannels {
		result = append(result, id)
	}
	return result
}

// AddWhitelistedDomain adds a domain to the whitelist.
func (f *Filter) AddWhitelistedDomain(domain string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whitelistDomains[strings.ToLower(domain)] = true
}

// RemoveWhitelistedDomain removes a domain from the whitelist.
func (f *Filter) RemoveWhitelistedDomain(domain string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	domain = strings.ToLower(domain)
	if f.whitelistDomains[domain] {
		delete(f.whitelistDomains, domain)
		return true
	}
	return false
}

// GetWhitelistedDomains returns all whitelisted domains.
func (f *Filter) GetWhitelistedDomains() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	result := make([]string, 0, len(f.whitelistDomains))
	for domain := range f.whitelistDomains {
		result = append(result, domain)
	}
	return result
}
