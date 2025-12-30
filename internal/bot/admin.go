// Package bot provides Telegram bot integration.
package bot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/xc9973/mogukefu-go/internal/keyword"
	"github.com/xc9973/mogukefu-go/internal/vector"
)

// AdminCommands handles admin bot commands.
type AdminCommands struct {
	bot      *TelegramBot
	deps     Dependencies
	adminIDs map[int64]bool
	logger   *slog.Logger
}

// NewAdminCommands creates a new AdminCommands handler.
func NewAdminCommands(bot *TelegramBot, deps Dependencies, adminIDs []int64, logger *slog.Logger) *AdminCommands {
	adminMap := make(map[int64]bool, len(adminIDs))
	for _, id := range adminIDs {
		adminMap[id] = true
	}

	return &AdminCommands{
		bot:      bot,
		deps:     deps,
		adminIDs: adminMap,
		logger:   logger,
	}
}

// isAdmin checks if a user is an admin.
// Implements Requirements 7.1
func (a *AdminCommands) isAdmin(userID int64) bool {
	return a.adminIDs[userID]
}

// HandleCommand routes commands to appropriate handlers.
func (a *AdminCommands) HandleCommand(ctx context.Context, msg *tgbotapi.Message, cmd, args string) {
	// Check admin permission for admin commands
	adminCommands := map[string]bool{
		"addkw": true, "delkw": true, "listkw": true,
		"addfaq": true, "delfaq": true, "listfaq": true, "showfaq": true,
		"exportkb": true, "importkb": true,
	}

	if adminCommands[cmd] && !a.isAdmin(msg.From.ID) {
		a.sendReply(msg, "⛔ 您没有权限执行此命令")
		return
	}

	switch cmd {
	case "start", "help":
		a.handleHelp(msg)
	case "addkw":
		a.handleAddKeyword(ctx, msg, args)
	case "delkw":
		a.handleDeleteKeyword(ctx, msg, args)
	case "listkw":
		a.handleListKeywords(ctx, msg)
	case "addfaq":
		a.handleAddFAQ(ctx, msg, args)
	case "delfaq":
		a.handleDeleteFAQ(ctx, msg, args)
	case "listfaq":
		a.handleListFAQs(ctx, msg)
	case "showfaq":
		a.handleShowFAQ(ctx, msg, args)
	case "exportkb":
		a.handleExportKB(ctx, msg)
	case "importkb":
		a.handleImportKB(ctx, msg, args)
	default:
		// Unknown command, ignore
	}
}

// handleHelp shows help message.
func (a *AdminCommands) handleHelp(msg *tgbotapi.Message) {
	help := `🤖 FAQ Bot 帮助

📝 关键词管理：
/addkw <关键词> | <回复> - 添加关键词
/delkw <关键词> - 删除关键词
/listkw - 列出所有关键词

📚 FAQ管理：
/addfaq <ID> | <问题> | <答案> - 添加FAQ
/delfaq <ID> - 删除FAQ
/listfaq - 列出所有FAQ
/showfaq <ID> - 查看FAQ详情

📦 知识库：
/exportkb - 导出知识库
/importkb <merge|overwrite> - 导入知识库`

	a.sendReply(msg, help)
}

// handleAddKeyword adds a new keyword.
// Implements Requirements 7.2
func (a *AdminCommands) handleAddKeyword(ctx context.Context, msg *tgbotapi.Message, args string) {
	// Parse: keyword | reply
	parts := strings.SplitN(args, "|", 2)
	if len(parts) != 2 {
		a.sendReply(msg, "❌ 格式错误\n用法: /addkw <关键词> | <回复>")
		return
	}

	kw := strings.TrimSpace(parts[0])
	reply := strings.TrimSpace(parts[1])

	if kw == "" || reply == "" {
		a.sendReply(msg, "❌ 关键词和回复不能为空")
		return
	}

	if err := a.deps.KBStore.AddKeyword(ctx, kw, reply); err != nil {
		a.logger.Error("failed to add keyword", "error", err, "keyword", kw)
		a.sendReply(msg, fmt.Sprintf("❌ 添加失败: %v", err))
		return
	}

	// Update keyword matcher cache
	a.refreshKeywordMatcher(ctx)

	a.sendReply(msg, fmt.Sprintf("✅ 关键词已添加: %s", kw))
}

// handleDeleteKeyword deletes a keyword.
// Implements Requirements 7.3
func (a *AdminCommands) handleDeleteKeyword(ctx context.Context, msg *tgbotapi.Message, args string) {
	kw := strings.TrimSpace(args)
	if kw == "" {
		a.sendReply(msg, "❌ 请指定要删除的关键词\n用法: /delkw <关键词>")
		return
	}

	if err := a.deps.KBStore.DeleteKeyword(ctx, kw); err != nil {
		a.logger.Error("failed to delete keyword", "error", err, "keyword", kw)
		a.sendReply(msg, fmt.Sprintf("❌ 删除失败: %v", err))
		return
	}

	// Update keyword matcher cache
	a.refreshKeywordMatcher(ctx)

	a.sendReply(msg, fmt.Sprintf("✅ 关键词已删除: %s", kw))
}

// handleListKeywords lists all keywords.
// Implements Requirements 7.4
func (a *AdminCommands) handleListKeywords(ctx context.Context, msg *tgbotapi.Message) {
	keywords, err := a.deps.KBStore.GetAllKeywords(ctx)
	if err != nil {
		a.logger.Error("failed to list keywords", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 获取关键词失败: %v", err))
		return
	}

	if len(keywords) == 0 {
		a.sendReply(msg, "📝 暂无关键词")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📝 关键词列表 (%d):\n\n", len(keywords)))
	for i, kw := range keywords {
		// Truncate long replies
		reply := kw.Reply
		if len(reply) > 50 {
			reply = reply[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. %s → %s\n", i+1, kw.Keyword, reply))
	}

	a.sendReply(msg, sb.String())
}

// handleAddFAQ adds a new FAQ.
// Implements Requirements 7.5
func (a *AdminCommands) handleAddFAQ(ctx context.Context, msg *tgbotapi.Message, args string) {
	// Parse: faq_id | question | answer
	parts := strings.SplitN(args, "|", 3)
	if len(parts) != 3 {
		a.sendReply(msg, "❌ 格式错误\n用法: /addfaq <ID> | <问题> | <答案>")
		return
	}

	faqID := strings.TrimSpace(parts[0])
	question := strings.TrimSpace(parts[1])
	answer := strings.TrimSpace(parts[2])

	if faqID == "" || question == "" || answer == "" {
		a.sendReply(msg, "❌ ID、问题和答案不能为空")
		return
	}

	// Generate embedding for the question
	a.sendReply(msg, "⏳ 正在生成向量...")

	vec, err := a.deps.EmbeddingClient.Embed(ctx, question)
	if err != nil {
		a.logger.Error("failed to generate embedding", "error", err, "faq_id", faqID)
		a.sendReply(msg, fmt.Sprintf("❌ 生成向量失败: %v", err))
		return
	}

	// Add FAQ with vector
	faq := vector.FAQEntry{
		FAQID:    faqID,
		Question: question,
		Answer:   answer,
	}

	if err := a.deps.VectorStore.AddFAQ(ctx, faq, vec); err != nil {
		a.logger.Error("failed to add FAQ", "error", err, "faq_id", faqID)
		a.sendReply(msg, fmt.Sprintf("❌ 添加FAQ失败: %v", err))
		return
	}

	a.sendReply(msg, fmt.Sprintf("✅ FAQ已添加: %s", faqID))
}

// handleDeleteFAQ deletes a FAQ.
// Implements Requirements 7.6
func (a *AdminCommands) handleDeleteFAQ(ctx context.Context, msg *tgbotapi.Message, args string) {
	faqID := strings.TrimSpace(args)
	if faqID == "" {
		a.sendReply(msg, "❌ 请指定要删除的FAQ ID\n用法: /delfaq <ID>")
		return
	}

	if err := a.deps.VectorStore.DeleteFAQ(ctx, faqID); err != nil {
		a.logger.Error("failed to delete FAQ", "error", err, "faq_id", faqID)
		a.sendReply(msg, fmt.Sprintf("❌ 删除失败: %v", err))
		return
	}

	a.sendReply(msg, fmt.Sprintf("✅ FAQ已删除: %s", faqID))
}

// handleListFAQs lists all FAQs.
// Implements Requirements 7.7
func (a *AdminCommands) handleListFAQs(ctx context.Context, msg *tgbotapi.Message) {
	faqs, err := a.deps.VectorStore.GetAllFAQs(ctx)
	if err != nil {
		a.logger.Error("failed to list FAQs", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 获取FAQ失败: %v", err))
		return
	}

	if len(faqs) == 0 {
		a.sendReply(msg, "📚 暂无FAQ")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📚 FAQ列表 (%d):\n\n", len(faqs)))
	for i, faq := range faqs {
		// Truncate long questions
		question := faq.Question
		if len(question) > 40 {
			question = question[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, faq.FAQID, question))
	}

	a.sendReply(msg, sb.String())
}

// handleShowFAQ shows FAQ details.
// Implements Requirements 7.8
func (a *AdminCommands) handleShowFAQ(ctx context.Context, msg *tgbotapi.Message, args string) {
	faqID := strings.TrimSpace(args)
	if faqID == "" {
		a.sendReply(msg, "❌ 请指定FAQ ID\n用法: /showfaq <ID>")
		return
	}

	faq, err := a.deps.VectorStore.GetFAQ(ctx, faqID)
	if err != nil {
		a.logger.Error("failed to get FAQ", "error", err, "faq_id", faqID)
		a.sendReply(msg, fmt.Sprintf("❌ 获取FAQ失败: %v", err))
		return
	}

	if faq == nil {
		a.sendReply(msg, fmt.Sprintf("❌ FAQ不存在: %s", faqID))
		return
	}

	text := fmt.Sprintf("📖 FAQ详情\n\nID: %s\n\n问题:\n%s\n\n答案:\n%s",
		faq.FAQID, faq.Question, faq.Answer)
	a.sendReply(msg, text)
}

// handleExportKB exports the knowledge base to YAML.
// Implements Requirements 7.9
func (a *AdminCommands) handleExportKB(ctx context.Context, msg *tgbotapi.Message) {
	yamlContent, err := a.deps.KBStore.ExportToYAML(ctx)
	if err != nil {
		a.logger.Error("failed to export KB", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 导出失败: %v", err))
		return
	}

	if yamlContent == "" || yamlContent == "keywords: []\nfaq: []\n" {
		a.sendReply(msg, "📦 知识库为空")
		return
	}

	// Send as document
	doc := tgbotapi.NewDocument(msg.Chat.ID, tgbotapi.FileBytes{
		Name:  "knowledge_base.yaml",
		Bytes: []byte(yamlContent),
	})

	if _, err := a.bot.GetAPI().Send(doc); err != nil {
		a.logger.Error("failed to send KB file", "error", err)
		// Fallback to text message if file sending fails
		if len(yamlContent) < 4000 {
			a.sendReply(msg, fmt.Sprintf("📦 知识库内容:\n```yaml\n%s\n```", yamlContent))
		} else {
			a.sendReply(msg, "❌ 发送文件失败，知识库内容过大")
		}
		return
	}

	a.sendReply(msg, "✅ 知识库已导出")
}

// handleImportKB imports the knowledge base from YAML.
// Implements Requirements 7.10
func (a *AdminCommands) handleImportKB(ctx context.Context, msg *tgbotapi.Message, args string) {
	mode := strings.TrimSpace(args)
	if mode != "merge" && mode != "overwrite" {
		a.sendReply(msg, "❌ 请指定导入模式\n用法: /importkb <merge|overwrite>\n\n请回复一个YAML文件")
		return
	}

	// Check if this is a reply to a document
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.Document == nil {
		a.sendReply(msg, "❌ 请回复一个YAML文件\n用法: 先发送YAML文件，然后回复该文件并输入 /importkb <merge|overwrite>")
		return
	}

	// Download the file
	fileID := msg.ReplyToMessage.Document.FileID
	file, err := a.bot.GetAPI().GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		a.logger.Error("failed to get file", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 获取文件失败: %v", err))
		return
	}

	// Download file content using HTTP client
	fileURL := file.Link(a.bot.GetAPI().Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		a.logger.Error("failed to download file", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 下载文件失败: %v", err))
		return
	}
	defer resp.Body.Close()

	// Read file content
	content, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB max
	if err != nil {
		a.logger.Error("failed to read file", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 读取文件失败: %v", err))
		return
	}
	yamlContent := string(content)

	// Import
	if err := a.deps.KBStore.ImportFromYAML(ctx, yamlContent, mode); err != nil {
		a.logger.Error("failed to import KB", "error", err)
		a.sendReply(msg, fmt.Sprintf("❌ 导入失败: %v", err))
		return
	}

	// Refresh caches
	a.refreshKeywordMatcher(ctx)
	if err := a.deps.VectorStore.RefreshCache(ctx); err != nil {
		a.logger.Warn("failed to refresh vector cache", "error", err)
	}

	a.sendReply(msg, fmt.Sprintf("✅ 知识库已导入 (模式: %s)\n⚠️ 注意: FAQ向量需要使用CLI工具重新生成", mode))
}

// refreshKeywordMatcher updates the keyword matcher with latest keywords.
func (a *AdminCommands) refreshKeywordMatcher(ctx context.Context) {
	keywords, err := a.deps.KBStore.GetAllKeywords(ctx)
	if err != nil {
		a.logger.Error("failed to refresh keywords", "error", err)
		return
	}

	// Convert to keyword.Entry
	entries := make([]keyword.Entry, len(keywords))
	for i, kw := range keywords {
		entries[i] = keyword.Entry{
			Keyword: kw.Keyword,
			Reply:   kw.Reply,
		}
	}

	a.deps.KeywordMatcher.UpdateKeywords(entries)
}

// sendReply sends a reply message.
func (a *AdminCommands) sendReply(msg *tgbotapi.Message, text string) {
	// Create a context with timeout for the reply
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := a.bot.sendReply(ctx, msg, text); err != nil {
		a.logger.Error("failed to send reply", "error", err)
	}
}
