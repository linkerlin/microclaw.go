package tools

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/linkerlin/microclaw.go/internal/db"
)

// ExportChatInput is the input for export_chat.
type ExportChatInput struct {
	Format string `json:"format,omitempty" jsonschema_description:"Output format: 'text' or 'markdown' (default: markdown)"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum number of messages (default: 100)"`
}

// ExportChatOutput is the output for export_chat.
type ExportChatOutput struct {
	Content      string `json:"content"`
	MessageCount int    `json:"message_count"`
}

// NewExportChatTool creates the export_chat tool.
func NewExportChatTool(database *db.DB, chatID int64, channel string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "export_chat",
		Description: "Export the chat history as text or markdown.",
	}, func(_ tool.Context, input ExportChatInput) (ExportChatOutput, error) {
		limit := input.Limit
		if limit <= 0 {
			limit = 100
		}
		format := input.Format
		if format == "" {
			format = "markdown"
		}

		msgs, err := database.GetMessages(chatID, channel, limit)
		if err != nil {
			return ExportChatOutput{}, fmt.Errorf("getting messages: %w", err)
		}

		var sb strings.Builder
		if format == "markdown" {
			sb.WriteString("# Chat Export\n\n")
		}

		for _, m := range msgs {
			ts := m.Timestamp.Format(time.RFC3339)
			sender := m.SenderName
			if m.IsFromBot {
				sender = "🤖 " + sender
			}
			if format == "markdown" {
				fmt.Fprintf(&sb, "**[%s] %s**: %s\n\n", ts, sender, m.Content)
			} else {
				fmt.Fprintf(&sb, "[%s] %s: %s\n", ts, sender, m.Content)
			}
		}

		return ExportChatOutput{
			Content:      sb.String(),
			MessageCount: len(msgs),
		}, nil
	})
}
