// Package model provides a multi-provider LLM bridge for microclaw.
// It implements model.LLM using OpenAI-compatible APIs and native Gemini.
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"

	"google.golang.org/adk/model"
)

// openAIBridge adapts OpenAI-compatible APIs to model.LLM.
type openAIBridge struct {
	name    string
	client  *openai.Client
	modelID string
}

// NewOpenAIBridge creates a model.LLM backed by an OpenAI-compatible endpoint.
// provider examples: anthropic, openai, openrouter, deepseek, ollama, openai-compatible.
func NewOpenAIBridge(name, modelID, baseURL, apiKey string) model.LLM {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &openAIBridge{
		name:    name,
		client:  openai.NewClientWithConfig(cfg),
		modelID: modelID,
	}
}

func (b *openAIBridge) Name() string {
	return b.name
}

// GenerateContent implements model.LLM.
func (b *openAIBridge) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		msgs, err := contentsToOpenAI(req.Contents)
		if err != nil {
			yield(nil, fmt.Errorf("converting contents: %w", err))
			return
		}

		chatReq := openai.ChatCompletionRequest{
			Model:    b.modelID,
			Messages: msgs,
		}

		// Apply config overrides.
		if req.Config != nil {
			if req.Config.MaxOutputTokens > 0 {
				chatReq.MaxTokens = int(req.Config.MaxOutputTokens)
			}
			if req.Config.Temperature != nil {
				chatReq.Temperature = float32(*req.Config.Temperature)
			}
			// Convert tools from genai format.
			if len(req.Config.Tools) > 0 {
				oaiTools, err := genaiToolsToOpenAI(req.Config.Tools)
				if err == nil {
					chatReq.Tools = oaiTools
					chatReq.ToolChoice = "auto"
				}
			}
		}

		resp, err := b.client.CreateChatCompletion(ctx, chatReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai api call: %w", err))
			return
		}
		if len(resp.Choices) == 0 {
			yield(nil, fmt.Errorf("empty response from api"))
			return
		}

		choice := resp.Choices[0]
		content, err := openAIChoiceToContent(choice)
		if err != nil {
			yield(nil, err)
			return
		}

		llmResp := &model.LLMResponse{
			Content:      content,
			TurnComplete: true,
		}
		if resp.Usage.TotalTokens > 0 {
			llmResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     int32(resp.Usage.PromptTokens),
				CandidatesTokenCount: int32(resp.Usage.CompletionTokens),
				TotalTokenCount:      int32(resp.Usage.TotalTokens),
			}
		}
		yield(llmResp, nil)
	}
}

// contentsToOpenAI converts genai.Content slice to OpenAI chat messages.
func contentsToOpenAI(contents []*genai.Content) ([]openai.ChatCompletionMessage, error) {
	var msgs []openai.ChatCompletionMessage
	for _, c := range contents {
		if c == nil {
			continue
		}
		role := mapRole(c.Role)
		var textParts []string
		var toolCalls []openai.ToolCall
		var toolResults []openai.ChatCompletionMessage

		for _, part := range c.Parts {
			if part == nil {
				continue
			}
			switch {
			case part.Text != "":
				textParts = append(textParts, part.Text)
			case part.FunctionCall != nil:
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, openai.ToolCall{
					ID:   part.FunctionCall.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			case part.FunctionResponse != nil:
				respJSON, _ := json.Marshal(part.FunctionResponse.Response)
				toolResults = append(toolResults, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: part.FunctionResponse.ID,
					Content:    string(respJSON),
				})
			}
		}

		if len(toolResults) > 0 {
			msgs = append(msgs, toolResults...)
			continue
		}

		msg := openai.ChatCompletionMessage{
			Role:    role,
			Content: strings.Join(textParts, "\n"),
		}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

func mapRole(role string) string {
	switch role {
	case "user":
		return openai.ChatMessageRoleUser
	case "model":
		return openai.ChatMessageRoleAssistant
	case "system":
		return openai.ChatMessageRoleSystem
	default:
		return openai.ChatMessageRoleUser
	}
}

// openAIChoiceToContent converts an OpenAI choice to genai.Content.
func openAIChoiceToContent(choice openai.ChatCompletionChoice) (*genai.Content, error) {
	var parts []*genai.Part

	if choice.Message.Content != "" {
		parts = append(parts, &genai.Part{Text: choice.Message.Content})
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"raw": tc.Function.Arguments}
		}
		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}

	if len(parts) == 0 {
		parts = []*genai.Part{{Text: ""}}
	}

	return &genai.Content{
		Role:  "model",
		Parts: parts,
	}, nil
}

// genaiToolsToOpenAI converts genai tool declarations to OpenAI tools format.
func genaiToolsToOpenAI(tools []*genai.Tool) ([]openai.Tool, error) {
	var result []openai.Tool
	for _, t := range tools {
		for _, fn := range t.FunctionDeclarations {
			params := map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
			if fn.Parameters != nil {
				paramsJSON, err := json.Marshal(fn.Parameters)
				if err == nil {
					var m map[string]interface{}
					if json.Unmarshal(paramsJSON, &m) == nil {
						params = m
					}
				}
			}
			result = append(result, openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  params,
				},
			})
		}
	}
	return result, nil
}
