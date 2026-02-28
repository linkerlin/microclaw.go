package tools

import (
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// EditFileInput is the input for edit_file (find-and-replace).
type EditFileInput struct {
	Path    string `json:"path" jsonschema_description:"Path to the file to edit"`
	OldText string `json:"old_text" jsonschema_description:"Exact text to find and replace"`
	NewText string `json:"new_text" jsonschema_description:"Replacement text"`
}

// EditFileOutput is the output for edit_file.
type EditFileOutput struct {
	Success      bool   `json:"success"`
	Replacements int    `json:"replacements"`
	Message      string `json:"message"`
}

// NewEditFileTool creates the edit_file tool.
func NewEditFileTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "edit_file",
		Description: "Find and replace text in a file. Replaces all occurrences of old_text with new_text.",
	}, func(_ tool.Context, input EditFileInput) (EditFileOutput, error) {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return EditFileOutput{}, fmt.Errorf("reading file: %w", err)
		}

		content := string(data)
		count := strings.Count(content, input.OldText)
		if count == 0 {
			return EditFileOutput{
				Success: false,
				Message: "old_text not found in file",
			}, nil
		}

		newContent := strings.ReplaceAll(content, input.OldText, input.NewText)
		if err := os.WriteFile(input.Path, []byte(newContent), 0o644); err != nil {
			return EditFileOutput{}, fmt.Errorf("writing file: %w", err)
		}

		return EditFileOutput{
			Success:      true,
			Replacements: count,
			Message:      fmt.Sprintf("replaced %d occurrence(s)", count),
		}, nil
	})
}
