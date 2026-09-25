//go:build windows || darwin

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ollama/ollama/auth"
)

type WebFetch struct{}

type FetchRequest struct {
	URL string `json:"url"`
}

type FetchResponse struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Links   []string `json:"links"`
}

func (w *WebFetch) Name() string {
	return "web_fetch"
}

func (w *WebFetch) Description() string {
	return "Crawl and extract text content from web pages"
}

func (g *WebFetch) Schema() map[string]any {
	schemaBytes := []byte(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL to crawl and extract content from"
            }
		},
		"required": ["url"]
	}`)
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil
	}
	return schema
}

func (w *WebFetch) Prompt() string {
	return ""
}

func (w *WebFetch) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	urlRaw, ok := args["url"]
	if !ok {
		return nil, "", fmt.Errorf("url parameter is required")
	}
	urlStr, ok := urlRaw.(string)
	if !ok || strings.TrimSpace(urlStr) == "" {
		return nil, "", fmt.Errorf("url must be a non-empty string")
	}

	result, err := performWebFetch(ctx, urlStr)
	if err != nil {
		return nil, "", err
	}
	for _, link := range result.Links {
		addAllowedDirectURL(ctx, link)
	}

	return result, "", nil
}

func performWebFetch(ctx context.Context, targetURL string) (*FetchResponse, error) {
	// Try Ollama cloud fetch first
	if err := ensureCloudEnabledForTool(ctx, "web fetch is unavailable"); err == nil {
		reqBody := FetchRequest{URL: targetURL}
		jsonBody, err := json.Marshal(reqBody)
		if err == nil {
			crawlURL, err := url.Parse("https://ollama.com/api/web_fetch")
			if err == nil {
				query := crawlURL.Query()
				query.Add("ts", strconv.FormatInt(time.Now().Unix(), 10))
				crawlURL.RawQuery = query.Encode()

				data := fmt.Appendf(nil, "%s,%s", http.MethodPost, crawlURL.RequestURI())
				signature, err := auth.Sign(ctx, data)
				if err == nil {
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, crawlURL.String(), bytes.NewBuffer(jsonBody))
					if err == nil {
						req.Header.Set("Content-Type", "application/json")
						if signature != "" {
							req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", signature))
						}
						client := &http.Client{Timeout: 15 * time.Second}
						resp, err := client.Do(req)
						if err == nil && resp.StatusCode == http.StatusOK {
							defer resp.Body.Close()
							var result FetchResponse
							if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
								return &result, nil
							}
						}
						if resp != nil {
							resp.Body.Close()
						}
					}
				}
			}
		}
	}

	// Fallback to direct HTTP fetch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL directly: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	rawText := string(bodyBytes)
	// Basic HTML cleanup
	cleanText := cleanHTMLContent(rawText)

	return &FetchResponse{
		Title:   targetURL,
		Content: cleanText,
		Links:   []string{},
	}, nil
}

func cleanHTMLContent(html string) string {
	// Simple text extraction by removing tags & scripts
	var b strings.Builder
	inTag := false
	inScript := false
	lower := strings.ToLower(html)

	for i := 0; i < len(html); i++ {
		if strings.HasPrefix(lower[i:], "<script") || strings.HasPrefix(lower[i:], "<style") {
			inScript = true
		}
		if inScript && strings.HasPrefix(lower[i:], "</script>") {
			inScript = false
			i += 8
			continue
		}
		if inScript && strings.HasPrefix(lower[i:], "</style>") {
			inScript = false
			i += 7
			continue
		}
		if inScript {
			continue
		}

		if html[i] == '<' {
			inTag = true
			continue
		}
		if html[i] == '>' {
			inTag = false
			b.WriteByte(' ')
			continue
		}
		if !inTag {
			b.WriteByte(html[i])
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
