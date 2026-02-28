package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// GrepInput is the input for grep_tool.
type GrepInput struct {
	Pattern   string `json:"pattern" jsonschema_description:"Regular expression pattern to search for"`
	Path      string `json:"path,omitempty" jsonschema_description:"File or directory to search in"`
	Recursive bool   `json:"recursive,omitempty" jsonschema_description:"Search recursively in directories"`
	MaxLines  int    `json:"max_lines,omitempty" jsonschema_description:"Maximum number of matching lines to return (default 100)"`
}

// GrepOutput is the output for grep_tool.
type GrepOutput struct {
	Matches []GrepMatch `json:"matches"`
	Count   int         `json:"count"`
}

// GrepMatch represents a single grep match.
type GrepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// NewGrepTool creates the grep search tool.
func NewGrepTool(workingDir string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "grep",
		Description: "Search for a regex pattern in files.",
	}, func(_ tool.Context, input GrepInput) (GrepOutput, error) {
		re, err := regexp.Compile(input.Pattern)
		if err != nil {
			return GrepOutput{}, fmt.Errorf("invalid pattern: %w", err)
		}

		maxLines := input.MaxLines
		if maxLines <= 0 {
			maxLines = 100
		}

		searchPath := workingDir
		if input.Path != "" {
			if filepath.IsAbs(input.Path) {
				searchPath = input.Path
			} else {
				searchPath = filepath.Join(workingDir, input.Path)
			}
		}

		var matches []GrepMatch

		err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if !input.Recursive && path != searchPath {
					return filepath.SkipDir
				}
				return nil
			}
			if len(matches) >= maxLines {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if len(matches) >= maxLines {
					break
				}
				if re.MatchString(line) {
					matches = append(matches, GrepMatch{
						File:    path,
						Line:    i + 1,
						Content: line,
					})
				}
			}
			return nil
		})
		if err != nil {
			return GrepOutput{}, err
		}

		return GrepOutput{
			Matches: matches,
			Count:   len(matches),
		}, nil
	})
}
