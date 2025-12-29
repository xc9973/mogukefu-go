// Package main provides the entry point for the Telegram FAQ Bot.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/xc9973/mogukefu-go/internal/bot"
	"github.com/xc9973/mogukefu-go/internal/config"
	"github.com/xc9973/mogukefu-go/internal/embedding"
	"github.com/xc9973/mogukefu-go/internal/handler"
	"github.com/xc9973/mogukefu-go/internal/kbstore"
	"github.com/xc9973/mogukefu-go/internal/keyword"
	"github.com/xc9973/mogukefu-go/internal/logger"
	"github.com/xc9973/mogukefu-go/internal/vector"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	dbPath := flag.String("db", "knowledge.db", "Path to SQLite database file")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", "text", "Log format (text, json)")
	flag.Parse()

	// Setup logger using the logger package
	// Implements Requirements 9.6
	log := logger.New(logger.Config{
		Level:  logger.ParseLevel(*logLevel),
		Format: *logFormat,
	})
	slog.SetDefault(log)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	cfg.SetDefaults()

	log.Info("configuration loaded", "config_path", *configPath)

	// Initialize database
	ctx := context.Background()
	store, err := kbstore.Open(*dbPath)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Initialize(ctx); err != nil {
		log.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	log.Info("database initialized", "db_path", *dbPath)


	// Load keywords from database
	keywords, err := store.GetAllKeywords(ctx)
	if err != nil {
		log.Error("failed to load keywords", "error", err)
		os.Exit(1)
	}

	// Convert to keyword.Entry
	keywordEntries := make([]keyword.Entry, len(keywords))
	for i, kw := range keywords {
		keywordEntries[i] = keyword.Entry{
			Keyword: kw.Keyword,
			Reply:   kw.Reply,
		}
	}

	// Initialize components
	keywordMatcher := keyword.NewMatcher(keywordEntries)
	log.Info("keyword matcher initialized", "keyword_count", len(keywordEntries))

	embeddingClient := embedding.NewOpenAIClient(embedding.Config{
		BaseURL: cfg.Embedding.BaseURL,
		APIKey:  cfg.Embedding.APIKey,
		Model:   cfg.Embedding.Model,
	})
	log.Info("embedding client initialized", "model", cfg.Embedding.Model)

	vectorStore := vector.New(store)
	if err := vectorStore.RefreshCache(ctx); err != nil {
		log.Error("failed to refresh vector cache", "error", err)
		os.Exit(1)
	}
	log.Info("vector store initialized")

	messageHandler := handler.New(
		keywordMatcher,
		vectorStore,
		embeddingClient,
		handler.Config{
			ShortMessageThreshold: cfg.Vector.ShortMessageThreshold,
			SimilarityThreshold:   cfg.Vector.SimilarityThreshold,
		},
	)
	log.Info("message handler initialized",
		"similarity_threshold", cfg.Vector.SimilarityThreshold,
		"short_message_threshold", cfg.Vector.ShortMessageThreshold,
	)

	// Create bot
	deps := bot.Dependencies{
		KBStore:         store,
		VectorStore:     vectorStore,
		EmbeddingClient: embeddingClient,
		KeywordMatcher:  keywordMatcher,
		MessageHandler:  messageHandler,
	}

	telegramBot, err := bot.NewTelegramBot(
		bot.Config{
			Token:                 cfg.Bot.Token,
			AdminIDs:              cfg.Bot.AdminIDs,
			SimilarityThreshold:   cfg.Vector.SimilarityThreshold,
			ShortMessageThreshold: cfg.Vector.ShortMessageThreshold,
		},
		deps,
		log,
	)
	if err != nil {
		log.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Info("received shutdown signal")
		if err := telegramBot.Stop(); err != nil {
			log.Error("failed to stop bot", "error", err)
		}
	}()

	// Run bot
	log.Info("starting bot...")
	if err := telegramBot.Run(); err != nil {
		log.Error("bot stopped with error", "error", err)
		os.Exit(1)
	}

	log.Info("bot stopped gracefully")
}
