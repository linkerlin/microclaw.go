package tools

import (
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ReadFileInput is the input for the read_file tool.
type ReadFileInput struct {
	Path   string `json:"path" jsonschema_description:"Path to the file to read"`
	Offset int    `json:"offset,omitempty" jsonschema_description:"Line number to start reading from (1-indexed, 0 means beginning)"`
	Limit  int    `json:"limit,omitempty" jsonschema_description:"Maximum number of lines to read (0 means all)"`
}

// ReadFileOutput is the output for the read_file tool.
type ReadFileOutput struct {
	Content    string `json:"content"`
	TotalLines int    `json:"total_lines"`
	LinesRead  int    `json:"lines_read"`
}

// NewReadFileTool creates the read_file tool.
func NewReadFileTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: "Read the contents of a file, optionally with line numbers, offset, and limit.",
	}, func(_ tool.Context, input ReadFileInput) (ReadFileOutput, error) {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return ReadFileOutput{}, fmt.Errorf("reading file: %w", err)
		}

		lines := strings.Split(string(data), "\n")
		total := len(lines)

		start := 0
		if input.Offset > 0 {
			start = input.Offset - 1
		}
		if start >= total {
			start = total
		}

		end := total
		if input.Limit > 0 {
			end = start + input.Limit
			if end > total {
				end = total
			}
		}

		selected := lines[start:end]
		var sb strings.Builder
		for i, line := range selected {
			fmt.Fprintf(&sb, "%d\t%s\n", start+i+1, line)
		}

		return ReadFileOutput{
			Content:    sb.String(),
			TotalLines: total,
			LinesRead:  len(selected),
		}, nil
	})
}
