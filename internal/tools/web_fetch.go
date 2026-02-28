package tools

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// WebFetchInput is the input for web_fetch.
type WebFetchInput struct {
	URL     string `json:"url" jsonschema_description:"URL to fetch"`
	MaxSize int    `json:"max_size,omitempty" jsonschema_description:"Maximum response size in bytes (default 100000)"`
}

// WebFetchOutput is the output for web_fetch.
type WebFetchOutput struct {
	Content     string `json:"content"`
	Title       string `json:"title"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	BytesRead   int    `json:"bytes_read"`
}

// NewWebFetchTool creates the web_fetch tool.
func NewWebFetchTool(maxDocSizeMB int) (tool.Tool, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	maxBytes := maxDocSizeMB * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}

	return functiontool.New(functiontool.Config{
		Name:        "web_fetch",
		Description: "Fetch a URL and return its text content with HTML stripped.",
	}, func(_ tool.Context, input WebFetchInput) (WebFetchOutput, error) {
		req, err := http.NewRequest("GET", input.URL, nil)
		if err != nil {
			return WebFetchOutput{}, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; microclaw/1.0)")

		resp, err := client.Do(req)
		if err != nil {
			return WebFetchOutput{}, fmt.Errorf("fetching URL: %w", err)
		}
		defer resp.Body.Close()

		limit := maxBytes
		if input.MaxSize > 0 && input.MaxSize < limit {
			limit = input.MaxSize
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
		if err != nil {
			return WebFetchOutput{}, fmt.Errorf("reading response: %w", err)
		}

		contentType := resp.Header.Get("Content-Type")
		var title, text string

		if strings.Contains(contentType, "html") {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
			if err == nil {
				title = doc.Find("title").First().Text()
				// Remove script and style elements.
				doc.Find("script, style, nav, footer, header").Remove()
				text = doc.Find("body").Text()
				// Clean up whitespace.
				lines := strings.Split(text, "\n")
				var cleaned []string
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" {
						cleaned = append(cleaned, trimmed)
					}
				}
				text = strings.Join(cleaned, "\n")
			} else {
				text = string(body)
			}
		} else {
			text = string(body)
		}

		return WebFetchOutput{
			Content:     text,
			Title:       title,
			StatusCode:  resp.StatusCode,
			ContentType: contentType,
			BytesRead:   len(body),
		}, nil
	})
}
