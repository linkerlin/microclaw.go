package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// BashInput is the input for the bash tool.
type BashInput struct {
	Command string `json:"command" jsonschema_description:"Shell command to execute"`
	Timeout int    `json:"timeout,omitempty" jsonschema_description:"Timeout in seconds (default 30)"`
}

// BashOutput is the output for the bash tool.
type BashOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// NewBashTool creates the bash execution tool.
func NewBashTool(workingDir string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "bash",
		Description: "Execute a shell command and return its output. Use for running scripts, file operations, and system tasks.",
	}, func(_ tool.Context, input BashInput) (BashOutput, error) {
		timeout := input.Timeout
		if timeout <= 0 {
			timeout = 30
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-c", input.Command)
		cmd.Dir = workingDir

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if ctx.Err() == context.DeadlineExceeded {
				return BashOutput{
					Stderr:   fmt.Sprintf("command timed out after %d seconds", timeout),
					ExitCode: -1,
				}, nil
			}
		}
		return BashOutput{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	})
}
