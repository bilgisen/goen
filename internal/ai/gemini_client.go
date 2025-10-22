package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilgisen/goen/internal/models"
	"github.com/go-resty/resty/v2"
)

type GeminiClient struct {
	client  *resty.Client
	apiKey  string
	model   string
	baseURL string
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	return &GeminiClient{
		client:  resty.New().SetTimeout(60 * time.Second),
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://generativelanguage.googleapis.com/v1beta/models",
	}
}

// GenerateEnglishNews processes a Turkish news item and returns an English version
func (g *GeminiClient) GenerateEnglishNews(ctx context.Context, item models.FeedItem) (*models.NewsItem, error) {
	// Build the prompt
	prompt := buildPrompt(item)

	// Call the Gemini API
	response, err := g.callGeminiAPI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("error calling Gemini API: %w", err)
	}

	// Parse the response into a NewsItem
	newsItem, err := parseGeminiResponse(response, item)
	if err != nil {
		return nil, fmt.Errorf("error parsing Gemini response: %w", err)
	}

	return newsItem, nil
}

func (g *GeminiClient) callGeminiAPI(ctx context.Context, prompt string) (string, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)

	req := geminiRequest{
		Contents: []geminiContent{{
			Parts: []geminiPart{{
				Text: prompt,
			}},
		}},
	}

	var resp geminiResponse
	_, err := g.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&resp).
		Post(url)

	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("API error: %s", resp.Error.Message)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return resp.Candidates[0].Content.Parts[0].Text, nil
}

func buildPrompt(item models.FeedItem) string {
	return fmt.Sprintf(`You are an expert English journalist and SEO writer. 
Transform this Turkish news article into a professional English version with the following structure:

1. SEO title (max 60 characters)
2. SEO description (max 160 characters)
3. TLDR (3 bullet points)
4. Main content in markdown format
5. Category (from the original)
6. Tags (5-7 relevant keywords)
7. Image title and description (for accessibility)

Respond in valid JSON format with these fields:
- seo_title
- seo_description
- tldr (array of strings)
- content_md (markdown formatted)
- category (from original)
- tags (array of strings)
- image_title
- image_description

Turkish Article:
Title: %s

Content: %s

Category: %s`, 
		escapeJSON(item.TitleTR), 
		escapeJSON(item.ContentTR), 
		escapeJSON(item.Category))
}

func parseGeminiResponse(response string, item models.FeedItem) (*models.NewsItem, error) {
	var result struct {
		SeoTitle    string   `json:"seo_title"`
		SeoDesc     string   `json:"seo_description"`
		TLDR        []string `json:	"tldr"`
		ContentMD   string   `json:"content_md"`
		Category    string   `json:"category"`
		Tags        []string `json:"tags"`
		ImageTitle  string   `json:"image_title"`
		ImageDesc   string   `json:"image_description"`
	}

	// Clean the response (sometimes Gemini adds markdown code blocks)
	cleanResponse := strings.TrimSpace(response)
	if strings.HasPrefix(cleanResponse, "```json") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```json\n")
		cleanResponse = strings.TrimSuffix(cleanResponse, "\n```")
	}

	if err := json.Unmarshal([]byte(cleanResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nResponse: %s", err, cleanResponse)
	}

	// Create and return the news item
	return &models.NewsItem{
		ID:          generateID(),
		SourceGuid:  item.Guid,
		SeoTitle:    result.SeoTitle,
		SeoDesc:     result.SeoDesc,
		TLDR:        result.TLDR,
		ContentMD:   result.ContentMD,
		Category:    result.Category,
		Tags:        result.Tags,
		ImageTitle:  result.ImageTitle,
		ImageDesc:   result.ImageDesc,
		OriginalUrl: item.Url,
		CreatedAt:   time.Now(),
	}, nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

func generateID() string {
	// In a real application, you might want to use UUID or another unique ID generator
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
