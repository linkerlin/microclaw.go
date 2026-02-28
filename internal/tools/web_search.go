package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// WebSearchInput is the input for web_search.
type WebSearchInput struct {
	Query   string `json:"query" jsonschema_description:"Search query"`
	NumResults int `json:"num_results,omitempty" jsonschema_description:"Number of results to return (default 5)"`
}

// WebSearchResult represents a single search result.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WebSearchOutput is the output for web_search.
type WebSearchOutput struct {
	Results []WebSearchResult `json:"results"`
	Query   string            `json:"query"`
}

// ddgSearchResponse is the DuckDuckGo JSON API response.
type ddgSearchResponse struct {
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	AbstractSource string `json:"AbstractSource"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
}

// NewWebSearchTool creates the web_search tool using DuckDuckGo.
func NewWebSearchTool() (tool.Tool, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	return functiontool.New(functiontool.Config{
		Name:        "web_search",
		Description: "Search the web using DuckDuckGo and return results.",
	}, func(_ tool.Context, input WebSearchInput) (WebSearchOutput, error) {
		numResults := input.NumResults
		if numResults <= 0 {
			numResults = 5
		}

		apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(input.Query) + "&format=json&no_html=1&skip_disambig=1"
		resp, err := client.Get(apiURL)
		if err != nil {
			return WebSearchOutput{}, fmt.Errorf("search request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return WebSearchOutput{}, fmt.Errorf("reading response: %w", err)
		}

		var ddg ddgSearchResponse
		if err := json.Unmarshal(body, &ddg); err != nil {
			return WebSearchOutput{}, fmt.Errorf("parsing response: %w", err)
		}

		var results []WebSearchResult
		if ddg.AbstractText != "" && ddg.AbstractURL != "" {
			results = append(results, WebSearchResult{
				Title:   ddg.AbstractSource,
				URL:     ddg.AbstractURL,
				Snippet: ddg.AbstractText,
			})
		}

		for _, topic := range ddg.RelatedTopics {
			if len(results) >= numResults {
				break
			}
			if topic.FirstURL == "" {
				continue
			}
			// Extract title from text (usually "Title - Description")
			title := topic.Text
			snippet := topic.Text
			if idx := strings.Index(topic.Text, " - "); idx > 0 {
				title = topic.Text[:idx]
				snippet = topic.Text[idx+3:]
			}
			results = append(results, WebSearchResult{
				Title:   title,
				URL:     topic.FirstURL,
				Snippet: snippet,
			})
		}

		return WebSearchOutput{
			Results: results,
			Query:   input.Query,
		}, nil
	})
}
