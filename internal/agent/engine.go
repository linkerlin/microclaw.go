// Package agent provides the core agentic loop for microclaw using adk-go.
package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/linkerlin/microclaw.go/internal/config"
	"github.com/linkerlin/microclaw.go/internal/db"
	"github.com/linkerlin/microclaw.go/internal/memory"
	localmodel "github.com/linkerlin/microclaw.go/internal/model"
	"github.com/linkerlin/microclaw.go/internal/tools"
)

const appName = "microclaw"

// Engine manages the agentic loop per chat.
type Engine struct {
	cfg        *config.Config
	db         *db.DB
	mem        *memory.Manager
	model      adkmodel.LLM
	sessionSvc session.Service
}

// New creates a new Engine with the given config, database, and memory manager.
func New(cfg *config.Config, database *db.DB, mem *memory.Manager) (*Engine, error) {
	m, err := buildModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("building model: %w", err)
	}

	return &Engine{
		cfg:        cfg,
		db:         database,
		mem:        mem,
		model:      m,
		sessionSvc: session.InMemoryService(),
	}, nil
}

// buildModel creates the appropriate model.LLM for the configured provider.
func buildModel(cfg *config.Config) (adkmodel.LLM, error) {
	provider := strings.ToLower(cfg.LLMProvider)

	if provider == "gemini" {
		ctx := context.Background()
		gcfg := &genai.ClientConfig{
			APIKey: cfg.APIKey,
		}
		m, err := newGeminiModel(ctx, cfg.Model, gcfg)
		if err != nil {
			return nil, fmt.Errorf("gemini model: %w", err)
		}
		return m, nil
	}

	// All other providers use OpenAI-compatible API.
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = config.BaseURLForProvider(provider)
	}
	return localmodel.NewOpenAIBridge(cfg.Model, cfg.Model, baseURL, cfg.APIKey), nil
}

// ChatRequest holds the input for processing a chat message.
type ChatRequest struct {
	ChatID      int64
	Channel     string
	UserName    string
	Message     string
	WorkingDir  string
}

// ChatResponse holds the output of processing a chat message.
type ChatResponse struct {
	Text string
}

// Process handles a single user message and returns the agent's response.
func (e *Engine) Process(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	sessionID := fmt.Sprintf("%s-%d", req.Channel, req.ChatID)
	userID := fmt.Sprintf("%s-%d", req.Channel, req.ChatID)

	// Ensure session exists.
	if _, err := e.sessionSvc.Get(ctx, &session.GetRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	}); err != nil {
		// Create new session.
		if _, err := e.sessionSvc.Create(ctx, &session.CreateRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		}); err != nil {
			return nil, fmt.Errorf("creating session: %w", err)
		}
	}

	// Build tools list.
	toolList, err := e.buildTools(req)
	if err != nil {
		return nil, fmt.Errorf("building tools: %w", err)
	}

	// Build system prompt.
	instruction := e.buildInstruction(req.ChatID)

	// Create agent.
	agentCfg := llmagent.Config{
		Name:        "microclaw",
		Model:       e.model,
		Description: "Microclaw AI assistant",
		Instruction: instruction,
		Tools:       toolList,
		GenerateContentConfig: &genai.GenerateContentConfig{
			MaxOutputTokens: int32(e.cfg.MaxTokens),
		},
	}

	a, err := llmagent.New(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          a,
		SessionService: e.sessionSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("creating runner: %w", err)
	}

	userMsg := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: req.Message}},
	}

	// Run the agentic loop.
	var responseText strings.Builder
	iterations := 0
	maxIter := e.cfg.MaxToolIterations

	for event, err := range r.Run(ctx, userID, sessionID, userMsg, agent.RunConfig{}) {
		if err != nil {
			log.Printf("agent error: %v", err)
			continue
		}
		if event == nil {
			continue
		}
		iterations++
		if iterations > maxIter {
			log.Printf("max tool iterations (%d) reached", maxIter)
			break
		}

		content := event.LLMResponse.Content
		if content == nil {
			continue
		}
		// Only collect final text responses (from model, not tool calls).
		if content.Role == "model" {
			for _, part := range content.Parts {
				if part != nil && part.Text != "" && part.FunctionCall == nil {
					responseText.WriteString(part.Text)
				}
			}
		}
	}

	return &ChatResponse{Text: responseText.String()}, nil
}



// buildInstruction constructs the system prompt from AGENTS.md + base instructions.
func (e *Engine) buildInstruction(chatID int64) string {
	base := `You are MicroClaw, a helpful AI assistant. You are running in a chat interface.
Be helpful, concise, and accurate. Use available tools when needed to complete tasks.
Today's date and time will be provided in context when available.`

	memContent := e.mem.ReadMemory(chatID)
	if memContent != "" {
		return base + "\n\n## Memory\n" + memContent
	}
	return base
}

// buildTools constructs the list of tools for the agent.
func (e *Engine) buildTools(req ChatRequest) ([]tool.Tool, error) {
	workingDir := e.resolveWorkingDir(req)

	var toolList []tool.Tool

	bash, err := tools.NewBashTool(workingDir)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, bash)

	readFile, err := tools.NewReadFileTool()
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, readFile)

	writeFile, err := tools.NewWriteFileTool()
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, writeFile)

	editFile, err := tools.NewEditFileTool()
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, editFile)

	glob, err := tools.NewGlobTool(workingDir)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, glob)

	grep, err := tools.NewGrepTool(workingDir)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, grep)

	webSearch, err := tools.NewWebSearchTool()
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, webSearch)

	webFetch, err := tools.NewWebFetchTool(e.cfg.MaxDocumentSizeMB)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, webFetch)

	readMem, err := tools.NewReadMemoryTool(e.mem, req.ChatID)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, readMem)

	writeMem, err := tools.NewWriteMemoryTool(e.mem, req.ChatID)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, writeMem)

	readTodo, err := tools.NewReadTodoTool(e.db, req.ChatID, req.Channel)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, readTodo)

	writeTodo, err := tools.NewWriteTodoTool(e.db, req.ChatID, req.Channel)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, writeTodo)

	addSchedule, err := tools.NewAddScheduleTool(e.db, req.ChatID, req.Channel)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, addSchedule)

	listSchedules, err := tools.NewListSchedulesTool(e.db, req.ChatID, req.Channel)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, listSchedules)

	deleteSchedule, err := tools.NewDeleteScheduleTool(e.db)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, deleteSchedule)

	exportChat, err := tools.NewExportChatTool(e.db, req.ChatID, req.Channel)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, exportChat)

	return toolList, nil
}

// resolveWorkingDir returns the working directory for a chat request.
func (e *Engine) resolveWorkingDir(req ChatRequest) string {
	if req.WorkingDir != "" {
		return req.WorkingDir
	}
	switch strings.ToLower(e.cfg.WorkingDirIsolation) {
	case "shared":
		return e.cfg.WorkingDir + "/shared"
	default:
		return fmt.Sprintf("%s/chat/%s/%d", e.cfg.WorkingDir, req.Channel, req.ChatID)
	}
}

// ClearSession removes session state for a chat.
func (e *Engine) ClearSession(ctx context.Context, chatID int64, channel string) error {
	sessionID := fmt.Sprintf("%s-%d", channel, chatID)
	userID := fmt.Sprintf("%s-%d", channel, chatID)
	return e.sessionSvc.Delete(ctx, &session.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
	})
}
