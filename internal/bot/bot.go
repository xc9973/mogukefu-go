// Package bot provides Telegram bot integration.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/xc9973/mogukefu-go/internal/antispam"
	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/handler"
	"github.com/xc9973/mogukefu-go/internal/kbstore"
	"github.com/xc9973/mogukefu-go/internal/keyword"
	"github.com/xc9973/mogukefu-go/internal/vector"
)

// Bot defines the interface for the Telegram bot.
type Bot interface {
	// Run starts the bot (blocking).
	Run() error

	// Stop stops the bot.
	Stop() error
}

// Config holds configuration for the bot.
type Config struct {
	Token                 string
	AdminIDs              []int64
	SimilarityThreshold   float64
	ShortMessageThreshold int
	
	// Antispam settings
	EnableAntispam         bool
	BlockForwardedChannels bool
	BlockExternalLinks     bool
	SpamKeywords           []string
	WhitelistedDomains     []string
	WhitelistedChannels    []int64
}

// Dependencies holds all dependencies for the bot.
type Dependencies struct {
	KBStore         kbstore.Store
	VectorStore     vector.Store
	EmbeddingClient embedding.Client
	KeywordMatcher  keyword.Matcher
	MessageHandler  handler.Handler
}

// TelegramBot implements the Bot interface using Telegram Bot API.
type TelegramBot struct {
	api           *tgbotapi.BotAPI
	config        Config
	deps          Dependencies
	logger        *slog.Logger
	stopCh        chan struct{}
	wg            sync.WaitGroup
	adminCommands *AdminCommands
	spamFilter    *antispam.Filter
}

// NewTelegramBot creates a new Telegram bot instance.
// Implements Requirements 5.1
func NewTelegramBot(cfg Config, deps Dependencies, logger *slog.Logger) (*TelegramBot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	bot := &TelegramBot{
		api:    api,
		config: cfg,
		deps:   deps,
		logger: logger,
		stopCh: make(chan struct{}),
	}

	// Initialize admin commands handler
	bot.adminCommands = NewAdminCommands(bot, deps, cfg.AdminIDs, logger)
	
	// Initialize spam filter if enabled
	if cfg.EnableAntispam {
		bot.spamFilter = antispam.NewFilter(antispam.Config{
			BlockForwardedChannels: cfg.BlockForwardedChannels,
			BlockExternalLinks:     cfg.BlockExternalLinks,
			BlockKeywords:          len(cfg.SpamKeywords) > 0,
			WhitelistedDomains:     cfg.WhitelistedDomains,
			WhitelistedUsers:       cfg.AdminIDs, // Admins bypass spam filter
			WhitelistedChannels:    cfg.WhitelistedChannels,
			SpamKeywords:           cfg.SpamKeywords,
		})
		logger.Info("antispam filter enabled",
			"block_forwarded", cfg.BlockForwardedChannels,
			"block_links", cfg.BlockExternalLinks,
			"spam_keywords", len(cfg.SpamKeywords),
			"whitelisted_channels", len(cfg.WhitelistedChannels),
		)
	}

	return bot, nil
}

// Run starts the bot and listens for messages using polling.
// Implements Requirements 5.1, 5.2, 5.3, 5.4
func (b *TelegramBot) Run() error {
	b.logger.Info("starting bot", "username", b.api.Self.UserName)

	// Configure update settings
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-b.stopCh:
			b.logger.Info("stopping bot")
			b.api.StopReceivingUpdates()
			b.wg.Wait()
			return nil

		case update := <-updates:
			if update.Message == nil {
				continue
			}

			b.wg.Add(1)
			go func(msg *tgbotapi.Message) {
				defer b.wg.Done()
				b.handleMessage(msg)
			}(update.Message)
		}
	}
}

// Stop stops the bot gracefully.
func (b *TelegramBot) Stop() error {
	close(b.stopCh)
	return nil
}

// handleMessage processes an incoming message.
// Implements Requirements 5.2, 5.3, 5.4, 5.5
func (b *TelegramBot) handleMessage(msg *tgbotapi.Message) {
	ctx := context.Background()

	// Log received message
	// Implements Requirements 9.1
	b.logger.Info("received message",
		"chat_id", msg.Chat.ID,
		"from_id", msg.From.ID,
		"text_length", len(msg.Text),
		"is_forwarded", msg.ForwardFromChat != nil,
	)

	// Check for spam/ads first (only in group chats)
	if b.spamFilter != nil && msg.Chat.Type != "private" {
		result := b.spamFilter.Check(msg)
		if result.IsSpam {
			b.logger.Warn("spam detected",
				"chat_id", msg.Chat.ID,
				"from_id", msg.From.ID,
				"reason", result.Reason,
			)
			
			// Try to delete the message
			if result.Action == antispam.ActionDelete {
				deleteMsg := tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID)
				if _, err := b.api.Request(deleteMsg); err != nil {
					b.logger.Error("failed to delete spam message",
						"error", err,
						"chat_id", msg.Chat.ID,
						"message_id", msg.MessageID,
					)
				} else {
					b.logger.Info("spam message deleted",
						"chat_id", msg.Chat.ID,
						"message_id", msg.MessageID,
						"reason", result.Reason,
					)
				}
			}
			return
		}
	}

	// Check if it's a command
	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	// Skip non-text messages
	if msg.Text == "" {
		return
	}

	// Process message through handler
	result, err := b.deps.MessageHandler.Handle(ctx, msg.Text)
	if err != nil {
		b.logger.Error("failed to handle message",
			"error", err,
			"chat_id", msg.Chat.ID,
		)
		return
	}

	// Log matching result
	// Implements Requirements 9.2, 9.3
	if result.MatchedKeyword != "" {
		b.logger.Info("keyword matched",
			"keyword", result.MatchedKeyword,
			"chat_id", msg.Chat.ID,
		)
	} else if result.MatchedFAQID != "" {
		b.logger.Info("FAQ matched",
			"faq_id", result.MatchedFAQID,
			"similarity", result.Similarity,
			"chat_id", msg.Chat.ID,
		)
	}

	// Send reply if needed
	// Implements Requirements 5.5
	if result.ShouldReply && result.ReplyText != "" {
		if err := b.sendReply(msg, result.ReplyText); err != nil {
			b.logger.Error("failed to send reply",
				"error", err,
				"chat_id", msg.Chat.ID,
			)
		} else {
			// Implements Requirements 9.4
			b.logger.Info("reply sent",
				"chat_id", msg.Chat.ID,
			)
		}
	}
}

// handleCommand processes bot commands.
func (b *TelegramBot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	cmd := msg.Command()
	args := msg.CommandArguments()

	b.logger.Info("received command",
		"command", cmd,
		"args", args,
		"from_id", msg.From.ID,
		"chat_id", msg.Chat.ID,
	)

	// Delegate to admin commands handler
	b.adminCommands.HandleCommand(ctx, msg, cmd, args)
}

// sendReply sends a reply message with exponential backoff retry.
// Implements Requirements 5.5, 5.6
func (b *TelegramBot) sendReply(originalMsg *tgbotapi.Message, text string) error {
	reply := tgbotapi.NewMessage(originalMsg.Chat.ID, text)
	reply.ReplyToMessageID = originalMsg.MessageID

	return b.sendWithRetry(reply)
}

// sendMessage sends a message to a chat.
func (b *TelegramBot) sendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	return b.sendWithRetry(msg)
}

// sendWithRetry sends a message with exponential backoff retry.
// Implements Requirements 5.6
func (b *TelegramBot) sendWithRetry(msg tgbotapi.MessageConfig) error {
	// Ensure text is valid UTF-8 before sending
	msg.Text = sanitizeUTF8(msg.Text)

	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		_, err := b.api.Send(msg)
		if err == nil {
			return nil
		}

		lastErr = err
		b.logger.Warn("send message failed, retrying",
			"attempt", attempt+1,
			"error", err,
		)
	}

	return fmt.Errorf("failed to send message after %d retries: %w", maxRetries, lastErr)
}

// sanitizeUTF8 ensures the string is valid UTF-8 by replacing invalid sequences.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	// Replace invalid UTF-8 sequences with the Unicode replacement character
	result := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		result = append(result, r)
		i += size
	}
	return string(result)
}

// GetAPI returns the underlying Telegram Bot API for admin commands.
func (b *TelegramBot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}

// GetSpamFilter returns the spam filter for admin commands.
func (b *TelegramBot) GetSpamFilter() *antispam.Filter {
	return b.spamFilter
}
