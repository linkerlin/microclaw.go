// Package telegram implements the Telegram channel for microclaw.
package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/linkerlin/microclaw.go/internal/agent"
	"github.com/linkerlin/microclaw.go/internal/config"
	"github.com/linkerlin/microclaw.go/internal/db"
	"github.com/linkerlin/microclaw.go/internal/memory"
)

const (
	channelName    = "telegram"
	maxMessageSize = 4096
)

// Channel is the Telegram bot channel.
type Channel struct {
	cfg    *config.Config
	db     *db.DB
	mem    *memory.Manager
	engine *agent.Engine
	bot    *tgbotapi.BotAPI
}

// New creates a new Telegram channel.
func New(cfg *config.Config, database *db.DB, mem *memory.Manager, eng *agent.Engine) (*Channel, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.Channels.Telegram.BotToken)
	if err != nil {
		return nil, fmt.Errorf("creating telegram bot: %w", err)
	}
	log.Printf("Telegram bot authorized as @%s", bot.Self.UserName)
	return &Channel{
		cfg:    cfg,
		db:     database,
		mem:    mem,
		engine: eng,
		bot:    bot,
	}, nil
}

// Run starts the Telegram update loop.
func (c *Channel) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := c.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			c.bot.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil {
				continue
			}
			go c.handleMessage(ctx, update.Message)
		}
	}
}

func (c *Channel) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID

	// Check permissions.
	if !c.isAllowed(chatID, userID) {
		log.Printf("Unauthorized access attempt from chat %d, user %d", chatID, userID)
		return
	}

	// Handle commands.
	if msg.IsCommand() {
		c.handleCommand(ctx, msg)
		return
	}

	// Process normal message.
	c.processMessage(ctx, msg)
}

func (c *Channel) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	cmd := msg.Command()

	switch cmd {
	case "start", "help":
		helpText := `MicroClaw AI Assistant

Commands:
/start, /help - Show this help
/clear - Clear conversation history
/skills - List available tools
/usage - Show usage info
/export - Export chat history`
		c.sendMessage(chatID, helpText)

	case "clear":
		if err := c.engine.ClearSession(ctx, chatID, channelName); err != nil {
			log.Printf("Error clearing session: %v", err)
		}
		if err := c.db.DeleteMessages(chatID, channelName); err != nil {
			log.Printf("Error clearing messages: %v", err)
		}
		c.sendMessage(chatID, "Conversation history cleared.")

	case "skills":
		skills := `Available tools:
• bash - Execute shell commands
• read_file, write_file, edit_file - File operations
• glob, grep - File search
• web_search - Search the web
• web_fetch - Fetch web pages
• read_memory, write_memory - Memory management
• read_todo, write_todo - Todo lists
• add_schedule, list_schedules, delete_schedule - Task scheduling
• export_chat - Export chat history`
		c.sendMessage(chatID, skills)

	case "usage":
		c.sendMessage(chatID, fmt.Sprintf("Model: %s\nProvider: %s", c.cfg.Model, c.cfg.LLMProvider))

	case "export":
		text := msg.CommandArguments()
		if text == "" {
			text = "markdown"
		}
		msgs, err := c.db.GetMessages(chatID, channelName, 100)
		if err != nil {
			c.sendMessage(chatID, "Error exporting chat.")
			return
		}
		var sb strings.Builder
		for _, m := range msgs {
			sender := m.SenderName
			if m.IsFromBot {
				sender = "Bot"
			}
			fmt.Fprintf(&sb, "[%s] %s: %s\n", m.Timestamp.Format("2006-01-02 15:04"), sender, m.Content)
		}
		if sb.Len() == 0 {
			c.sendMessage(chatID, "No messages to export.")
		} else {
			c.sendLongMessage(chatID, sb.String())
		}
	}
}

func (c *Channel) processMessage(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := msg.Text
	if text == "" {
		return
	}

	// Save user message.
	_ = c.db.SaveMessage(&db.Message{
		ChatID:      chatID,
		ChatChannel: channelName,
		SenderName:  msg.From.UserName,
		Content:     text,
		IsFromBot:   false,
	})

	// Send typing indicator.
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = c.bot.Send(typing)

	// Process through agent engine.
	resp, err := c.engine.Process(ctx, agent.ChatRequest{
		ChatID:   chatID,
		Channel:  channelName,
		UserName: msg.From.UserName,
		Message:  text,
	})
	if err != nil {
		log.Printf("Error processing message: %v", err)
		c.sendMessage(chatID, "Sorry, I encountered an error processing your message.")
		return
	}

	if resp.Text == "" {
		resp.Text = "I couldn't generate a response."
	}

	// Save bot response.
	_ = c.db.SaveMessage(&db.Message{
		ChatID:      chatID,
		ChatChannel: channelName,
		SenderName:  c.cfg.Channels.Telegram.BotUsername,
		Content:     resp.Text,
		IsFromBot:   true,
	})

	c.sendLongMessage(chatID, resp.Text)
}

// isAllowed checks if a chat/user is permitted to use the bot.
func (c *Channel) isAllowed(chatID, userID int64) bool {
	tgCfg := c.cfg.Channels.Telegram
	// If no restrictions, allow all.
	if len(tgCfg.AllowedGroups) == 0 && len(tgCfg.AllowedUserIDs) == 0 {
		return true
	}
	for _, id := range tgCfg.AllowedGroups {
		if id == chatID {
			return true
		}
	}
	for _, id := range tgCfg.AllowedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (c *Channel) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := c.bot.Send(msg); err != nil {
		// Retry without markdown formatting.
		msg.ParseMode = ""
		_, _ = c.bot.Send(msg)
	}
}

func (c *Channel) sendLongMessage(chatID int64, text string) {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxMessageSize {
			// Find a good split point.
			idx := strings.LastIndex(text[:maxMessageSize], "\n")
			if idx < maxMessageSize/2 {
				idx = maxMessageSize
			}
			chunk = text[:idx]
			text = text[idx:]
		} else {
			text = ""
		}
		c.sendMessage(chatID, chunk)
	}
}
