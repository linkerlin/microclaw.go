package tools

import (
	"path/filepath"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// GlobInput is the input for glob_tool.
type GlobInput struct {
	Pattern string `json:"pattern" jsonschema_description:"Glob pattern to match files (e.g., '**/*.go')"`
	BaseDir string `json:"base_dir,omitempty" jsonschema_description:"Base directory to search in (default: current directory)"`
}

// GlobOutput is the output for glob_tool.
type GlobOutput struct {
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

// NewGlobTool creates the glob file-finding tool.
func NewGlobTool(workingDir string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "glob",
		Description: "Find files matching a glob pattern.",
	}, func(_ tool.Context, input GlobInput) (GlobOutput, error) {
		base := workingDir
		if input.BaseDir != "" {
			base = input.BaseDir
		}

		pattern := filepath.Join(base, input.Pattern)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return GlobOutput{}, err
		}
		if matches == nil {
			matches = []string{}
		}
		return GlobOutput{
			Matches: matches,
			Count:   len(matches),
		}, nil
	})
}
