package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/linkerlin/microclaw.go/internal/db"
)

// ReadTodoInput is the input for read_todo.
type ReadTodoInput struct{}

// ReadTodoOutput is the output for read_todo.
type ReadTodoOutput struct {
	Content string `json:"content"`
}

// WriteTodoInput is the input for write_todo.
type WriteTodoInput struct {
	Content string `json:"content" jsonschema_description:"Todo list content in Markdown format"`
}

// WriteTodoOutput is the output for write_todo.
type WriteTodoOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewReadTodoTool creates the read_todo tool.
func NewReadTodoTool(database *db.DB, chatID int64, channel string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_todo",
		Description: "Read the current todo list for this chat.",
	}, func(_ tool.Context, _ ReadTodoInput) (ReadTodoOutput, error) {
		content, err := database.GetTodo(chatID, channel)
		if err != nil {
			return ReadTodoOutput{}, err
		}
		return ReadTodoOutput{Content: content}, nil
	})
}

// NewWriteTodoTool creates the write_todo tool.
func NewWriteTodoTool(database *db.DB, chatID int64, channel string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "write_todo",
		Description: "Update the todo list for this chat.",
	}, func(_ tool.Context, input WriteTodoInput) (WriteTodoOutput, error) {
		if err := database.SaveTodo(chatID, channel, input.Content); err != nil {
			return WriteTodoOutput{Success: false, Message: err.Error()}, nil
		}
		return WriteTodoOutput{Success: true, Message: "todo list updated"}, nil
	})
}
