package agent

import (
	"context"

	"google.golang.org/genai"

	adkgemini "google.golang.org/adk/model/gemini"
	adkmodel "google.golang.org/adk/model"
)

// newGeminiModel creates a Gemini model using adk-go's built-in implementation.
func newGeminiModel(ctx context.Context, modelName string, cfg *genai.ClientConfig) (adkmodel.LLM, error) {
	return adkgemini.NewModel(ctx, modelName, cfg)
}
