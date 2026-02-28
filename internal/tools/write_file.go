package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// WriteFileInput is the input for write_file.
type WriteFileInput struct {
	Path    string `json:"path" jsonschema_description:"Path to write the file to"`
	Content string `json:"content" jsonschema_description:"Content to write to the file"`
}

// WriteFileOutput is the output for write_file.
type WriteFileOutput struct {
	BytesWritten int    `json:"bytes_written"`
	Path         string `json:"path"`
}

// NewWriteFileTool creates the write_file tool.
func NewWriteFileTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content.",
	}, func(_ tool.Context, input WriteFileInput) (WriteFileOutput, error) {
		if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
			return WriteFileOutput{}, fmt.Errorf("creating directories: %w", err)
		}
		if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
			return WriteFileOutput{}, fmt.Errorf("writing file: %w", err)
		}
		return WriteFileOutput{
			BytesWritten: len(input.Content),
			Path:         input.Path,
		}, nil
	})
}
