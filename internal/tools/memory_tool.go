package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/linkerlin/microclaw.go/internal/memory"
)

// ReadMemoryInput is the input for read_memory.
type ReadMemoryInput struct {
	Scope string `json:"scope,omitempty" jsonschema_description:"'global' or 'chat' (default: both)"`
}

// ReadMemoryOutput is the output for read_memory.
type ReadMemoryOutput struct {
	Content string `json:"content"`
}

// WriteMemoryInput is the input for write_memory.
type WriteMemoryInput struct {
	Content string `json:"content" jsonschema_description:"Memory content to write (Markdown format)"`
	Global  bool   `json:"global,omitempty" jsonschema_description:"Write to global memory (true) or chat-specific memory (false)"`
}

// WriteMemoryOutput is the output for write_memory.
type WriteMemoryOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewReadMemoryTool creates the read_memory tool.
func NewReadMemoryTool(mem *memory.Manager, chatID int64) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_memory",
		Description: "Read the AGENTS.md memory files (global and/or chat-specific).",
	}, func(_ tool.Context, _ ReadMemoryInput) (ReadMemoryOutput, error) {
		content := mem.ReadMemory(chatID)
		return ReadMemoryOutput{Content: content}, nil
	})
}

// NewWriteMemoryTool creates the write_memory tool.
func NewWriteMemoryTool(mem *memory.Manager, chatID int64) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "write_memory",
		Description: "Write or update the AGENTS.md memory. Set global=true to update global memory.",
	}, func(_ tool.Context, input WriteMemoryInput) (WriteMemoryOutput, error) {
		if err := mem.WriteMemory(chatID, input.Content, input.Global); err != nil {
			return WriteMemoryOutput{Success: false, Message: err.Error()}, nil
		}
		scope := "chat"
		if input.Global {
			scope = "global"
		}
		return WriteMemoryOutput{
			Success: true,
			Message: "memory updated (" + scope + ")",
		}, nil
	})
}
